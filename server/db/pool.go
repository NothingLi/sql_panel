// Package db 提供数据库连接池管理和系统数据库初始化功能。
package db

import (
	"database/sql"
	"fmt"
	"sql_panel/server/models"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// connPool 数据库连接池，管理所有用户创建的数据库连接和事务。
// 使用读写锁保证并发安全。
type connPool struct {
	mu   sync.RWMutex
	pool map[string]*sql.DB // 连接池，key 为 connection ID
	txs  map[string]*sql.Tx // 活跃事务，key 为 "connID:userID"
}

// Pool 全局连接池实例。
var Pool = &connPool{
	pool: make(map[string]*sql.DB),
	txs:  make(map[string]*sql.Tx),
}

// txKey 生成事务的唯一标识键。
func txKey(connID string, userID int) string {
	return fmt.Sprintf("%s:%d", connID, userID)
}

// GetConnection 获取或创建数据库连接。
// 优先从连接池复用已有连接，连接失效时自动重建。
func (p *connPool) GetConnection(connInfo models.Connection) (*sql.DB, error) {
	// 先用读锁检查连接池中是否已有可用连接
	p.mu.RLock()
	if db, ok := p.pool[connInfo.ID]; ok {
		p.mu.RUnlock()
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// 连接已失效，清理后重建
		db.Close()
		p.mu.Lock()
		delete(p.pool, connInfo.ID)
		p.mu.Unlock()
	} else {
		p.mu.RUnlock()
	}

	// 写锁保护下创建新连接
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getConnectionLocked(connInfo)
}

// RemoveConnection 移除指定连接并回滚其所有活跃事务。
func (p *connPool) RemoveConnection(connID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if db, ok := p.pool[connID]; ok {
		db.Close()
		delete(p.pool, connID)
	}

	// 回滚该连接的所有活跃事务
	for key, tx := range p.txs {
		if strings.HasPrefix(key, connID+":") {
			tx.Rollback()
			delete(p.txs, key)
		}
	}
}

// BeginTx 开启一个新事务。如果已有活跃事务则先回滚旧事务。
func (p *connPool) BeginTx(connInfo models.Connection, userID int) (*sql.Tx, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := txKey(connInfo.ID, userID)
	// 如果已有活跃事务，先回滚
	if tx, ok := p.txs[key]; ok {
		tx.Rollback()
	}

	db, err := p.getConnectionLocked(connInfo)
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	p.txs[key] = tx
	return tx, nil
}

// GetTx 获取当前活跃事务，不存在则返回 nil。
func (p *connPool) GetTx(connID string, userID int) *sql.Tx {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.txs[txKey(connID, userID)]
}

// CommitTx 提交事务并从事务表中移除。
func (p *connPool) CommitTx(connID string, userID int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := txKey(connID, userID)
	tx, ok := p.txs[key]
	if !ok {
		return fmt.Errorf("no active transaction")
	}
	delete(p.txs, key)
	return tx.Commit()
}

// RollbackTx 回滚事务并从事务表中移除。
func (p *connPool) RollbackTx(connID string, userID int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := txKey(connID, userID)
	tx, ok := p.txs[key]
	if !ok {
		return fmt.Errorf("no active transaction")
	}
	delete(p.txs, key)
	return tx.Rollback()
}

// getConnectionLocked 在持有写锁的情况下创建数据库连接。
// 根据数据库类型构建对应的 DSN 并建立连接。
func (p *connPool) getConnectionLocked(connInfo models.Connection) (*sql.DB, error) {
	// 双重检查：可能在等待写锁期间其他协程已创建连接
	if db, ok := p.pool[connInfo.ID]; ok {
		if err := db.Ping(); err == nil {
			return db, nil
		}
		db.Close()
		delete(p.pool, connInfo.ID)
	}

	// 根据数据库类型构建驱动名和 DSN
	var driverName, dsn string
	switch connInfo.Type {
	case models.SQLite:
		driverName = "sqlite"
		dsn = connInfo.Host
	case models.MySQL:
		driverName = "mysql"
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?multiStatements=true",
			connInfo.User, connInfo.Password, connInfo.Host, connInfo.Port, connInfo.DatabaseName)
	case models.PostgreSQL:
		driverName = "postgres"
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
			connInfo.User, connInfo.Password, connInfo.Host, connInfo.Port, connInfo.DatabaseName)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", connInfo.Type)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	// 配置连接池参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 验证连接可用性
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	p.pool[connInfo.ID] = db
	return db, nil
}

// CloseAll 关闭所有连接并清空连接池。
func (p *connPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, db := range p.pool {
		db.Close()
		delete(p.pool, id)
	}
}
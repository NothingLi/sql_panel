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
	mu       sync.RWMutex
	pool     map[string]*sql.DB // 连接池，key 为 connection ID
	txs      map[string]*sql.Tx // 活跃事务，key 为 "connID:userID"
	versions map[string]string  // 版本缓存，key 为 connection ID，value 为版本字符串
}

// Pool 全局连接池实例。
var Pool = &connPool{
	pool:     make(map[string]*sql.DB),
	txs:      make(map[string]*sql.Tx),
	versions: make(map[string]string),
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
	delete(p.versions, connID)

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

// GetVersion 探测并缓存数据库版本。
// 优先返回缓存值，未缓存时执行版本探测 SQL 并解析主版本号。
// 返回的主版本号用于前端函数库过滤，例如 PostgreSQL 14.5 返回 "14"。
func (p *connPool) GetVersion(connInfo models.Connection) (string, error) {
	// 先查缓存
	p.mu.RLock()
	if v, ok := p.versions[connInfo.ID]; ok {
		p.mu.RUnlock()
		return v, nil
	}
	p.mu.RUnlock()

	dbConn, err := p.GetConnection(connInfo)
	if err != nil {
		return "", err
	}

	var versionSQL, raw string
	switch connInfo.Type {
	case models.PostgreSQL:
		versionSQL = "SELECT version()"
	case models.MySQL:
		versionSQL = "SELECT VERSION()"
	case models.SQLite:
		versionSQL = "SELECT sqlite_version()"
	case models.SQLServer:
		versionSQL = "SELECT @@VERSION"
	default:
		return "", fmt.Errorf("unsupported database type: %s", connInfo.Type)
	}

	if err := dbConn.QueryRow(versionSQL).Scan(&raw); err != nil {
		return "", err
	}

	major := parseMajorVersion(raw, connInfo.Type)

	p.mu.Lock()
	p.versions[connInfo.ID] = major
	p.mu.Unlock()
	return major, nil
}

// parseMajorVersion 从版本字符串中解析主版本号。
// 不同数据库的 version() 输出格式差异较大，这里按类型提取主版本号。
func parseMajorVersion(raw string, dbType models.DBType) string {
	fields := strings.Fields(raw)
	var numeric string
	for _, f := range fields {
		// 找到第一个以数字开头的 token
		if len(f) > 0 && f[0] >= '0' && f[0] <= '9' {
			numeric = f
			break
		}
	}
	if numeric == "" {
		return ""
	}
	// 取第一个点之前的部分作为主版本号
	if idx := strings.Index(numeric, "."); idx > 0 {
		return numeric[:idx]
	}
	return numeric
}

// CloseAll 关闭所有连接并清空连接池。
func (p *connPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, db := range p.pool {
		db.Close()
		delete(p.pool, id)
	}
	for id := range p.versions {
		delete(p.versions, id)
	}
}

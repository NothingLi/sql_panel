// Package db 提供系统数据库初始化、表创建、迁移和默认数据填充功能。
package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// DB 全局系统数据库实例，存储用户、连接配置等系统数据。
var DB *sql.DB

// InitDB 初始化系统数据库。
// 通过环境变量 SYSTEM_DB_TYPE 选择数据库类型（sqlite/mysql/postgres），
// 自动创建表结构并执行迁移。
func InitDB() {
	dbType := os.Getenv("SYSTEM_DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}

	var driverName, dsn string

	switch dbType {
	case "mysql":
		driverName = "mysql"
		dsn = os.Getenv("SYSTEM_DB_DSN")
		if dsn == "" {
			log.Fatal("SYSTEM_DB_DSN is required when SYSTEM_DB_TYPE=mysql")
		}
	case "postgres":
		driverName = "postgres"
		dsn = os.Getenv("SYSTEM_DB_DSN")
		if dsn == "" {
			log.Fatal("SYSTEM_DB_DSN is required when SYSTEM_DB_TYPE=postgres")
		}
	default:
		driverName = "sqlite"
		dsn = "./panel.db"
	}

	var err error
	DB, err = sql.Open(driverName, dsn)
	if err != nil {
		log.Fatal("Failed to open system database:", err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatal("Failed to ping system database:", err)
	}

	// 自动创建表，失败时打印手动建表 SQL
	if err := createTables(dbType); err != nil {
		log.Printf("WARNING: Failed to auto-create tables: %v", err)
		log.Printf("The database user may lack DDL privileges.")
		log.Printf("Please run the following SQL manually with a privileged account:\n\n%s\n", buildInitSQL(dbType))
	} else {
		runMigrations(dbType)
		ensureAdminUser(dbType)
	}

	fmt.Printf("System database initialized successfully (%s)\n", dbType)
}

// createTables 根据数据库类型创建所有系统表。
func createTables(dbType string) error {
	tables := getTableDDLs(dbType)
	for name, ddl := range tables {
		if _, err := DB.Exec(ddl); err != nil {
			return fmt.Errorf("create table %s: %w", name, err)
		}
	}
	return nil
}

// runMigrations 执行数据库迁移，添加 shared_by 列并回填数据。
func runMigrations(dbType string) {
	// 根据数据库类型构建 ALTER TABLE 语句
	addColumnSQL := ""
	switch dbType {
	case "mysql":
		addColumnSQL = `ALTER TABLE shared_connections ADD COLUMN shared_by INT`
	case "postgres":
		addColumnSQL = `ALTER TABLE shared_connections ADD COLUMN IF NOT EXISTS shared_by INT`
	default:
		addColumnSQL = `ALTER TABLE shared_connections ADD COLUMN shared_by INTEGER`
	}

	// 列可能已存在，忽略错误
	if _, err := DB.Exec(addColumnSQL); err != nil {
		return
	}

	// 回填 shared_by 字段为连接的创建者
	backfillSQL := `UPDATE shared_connections SET shared_by = (
		SELECT creator_id FROM connections WHERE connections.id = shared_connections.connection_id
	) WHERE shared_by IS NULL`
	DB.Exec(backfillSQL)
}

// ensureAdminUser 确保默认 admin 用户存在（密码为 admin123 的 bcrypt 哈希）。
func ensureAdminUser(dbType string) {
	adminPassword := "$2a$10$1t59Wgfp8bfaFEwKBn0ziOWraWrhS5MG1Dmf3/HEXkPWmVrVaXvlq"

	var upsertSQL string
	switch dbType {
	case "mysql":
		upsertSQL = `INSERT IGNORE INTO users (username, password, role) VALUES ('admin', ?, 'admin')`
	case "postgres":
		upsertSQL = `INSERT INTO users (username, password, role) VALUES ('admin', ?, 'admin') ON CONFLICT (username) DO NOTHING`
	default:
		upsertSQL = `INSERT OR IGNORE INTO users (username, password, role) VALUES ('admin', ?, 'admin')`
	}

	if _, err := DB.Exec(upsertSQL, adminPassword); err != nil {
		log.Printf("WARNING: Failed to ensure admin user: %v", err)
	}
}

// getTableDDLs 返回各数据库类型对应的建表 DDL 语句。
func getTableDDLs(dbType string) map[string]string {
	switch dbType {
	case "mysql":
		return map[string]string{
			"users": `CREATE TABLE IF NOT EXISTS users (
				id INT AUTO_INCREMENT PRIMARY KEY,
				username VARCHAR(255) NOT NULL UNIQUE,
				password VARCHAR(255) NOT NULL,
				role VARCHAR(50) NOT NULL DEFAULT 'user'
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			"connections": `CREATE TABLE IF NOT EXISTS connections (
				id VARCHAR(36) PRIMARY KEY,
				creator_id INT NOT NULL,
				name VARCHAR(255) NOT NULL,
				type VARCHAR(50) NOT NULL,
				host VARCHAR(255) NOT NULL,
				port INT,
				db_user VARCHAR(255),
				db_password VARCHAR(255),
				database_name VARCHAR(255),
				FOREIGN KEY(creator_id) REFERENCES users(id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			"shared_connections": `CREATE TABLE IF NOT EXISTS shared_connections (
				user_id INT NOT NULL,
				connection_id VARCHAR(36) NOT NULL,
				shared_by INT,
				PRIMARY KEY(user_id, connection_id),
				FOREIGN KEY(user_id) REFERENCES users(id),
				FOREIGN KEY(connection_id) REFERENCES connections(id) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			"export_logs": `CREATE TABLE IF NOT EXISTS export_logs (
				id INT AUTO_INCREMENT PRIMARY KEY,
				user_id INT NOT NULL,
				username VARCHAR(255) NOT NULL,
				connection_name VARCHAR(255) NOT NULL DEFAULT '',
				sql_statement TEXT NOT NULL,
				row_count INT NOT NULL DEFAULT 0,
				file_type VARCHAR(50) NOT NULL DEFAULT '',
				file_name VARCHAR(255) NOT NULL DEFAULT '',
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY(user_id) REFERENCES users(id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
	case "postgres":
		return map[string]string{
			"users": `CREATE TABLE IF NOT EXISTS users (
				id SERIAL PRIMARY KEY,
				username VARCHAR(255) NOT NULL UNIQUE,
				password VARCHAR(255) NOT NULL,
				role VARCHAR(50) NOT NULL DEFAULT 'user'
			)`,
			"connections": `CREATE TABLE IF NOT EXISTS connections (
				id VARCHAR(36) PRIMARY KEY,
				creator_id INT NOT NULL,
				name VARCHAR(255) NOT NULL,
				type VARCHAR(50) NOT NULL,
				host VARCHAR(255) NOT NULL,
				port INT,
				db_user VARCHAR(255),
				db_password VARCHAR(255),
				database_name VARCHAR(255),
				FOREIGN KEY(creator_id) REFERENCES users(id)
			)`,
			"shared_connections": `CREATE TABLE IF NOT EXISTS shared_connections (
				user_id INT NOT NULL,
				connection_id VARCHAR(36) NOT NULL,
				shared_by INT,
				PRIMARY KEY(user_id, connection_id),
				FOREIGN KEY(user_id) REFERENCES users(id),
				FOREIGN KEY(connection_id) REFERENCES connections(id) ON DELETE CASCADE
			)`,
			"export_logs": `CREATE TABLE IF NOT EXISTS export_logs (
				id SERIAL PRIMARY KEY,
				user_id INT NOT NULL,
				username VARCHAR(255) NOT NULL,
				connection_name VARCHAR(255) NOT NULL DEFAULT '',
				sql_statement TEXT NOT NULL DEFAULT '',
				row_count INT NOT NULL DEFAULT 0,
				file_type VARCHAR(50) NOT NULL DEFAULT '',
				file_name VARCHAR(255) NOT NULL DEFAULT '',
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY(user_id) REFERENCES users(id)
			)`,
		}
	default:
		// SQLite 为默认数据库类型
		return map[string]string{
			"users": `CREATE TABLE IF NOT EXISTS users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT UNIQUE NOT NULL,
				password TEXT NOT NULL,
				role TEXT NOT NULL DEFAULT 'user'
			)`,
			"connections": `CREATE TABLE IF NOT EXISTS connections (
				id TEXT PRIMARY KEY,
				creator_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				type TEXT NOT NULL,
				host TEXT NOT NULL,
				port INTEGER,
				db_user TEXT,
				db_password TEXT,
				database_name TEXT,
				FOREIGN KEY(creator_id) REFERENCES users(id)
			)`,
			"shared_connections": `CREATE TABLE IF NOT EXISTS shared_connections (
				user_id INTEGER NOT NULL,
				connection_id TEXT NOT NULL,
				shared_by INTEGER,
				PRIMARY KEY(user_id, connection_id),
				FOREIGN KEY(user_id) REFERENCES users(id),
				FOREIGN KEY(connection_id) REFERENCES connections(id) ON DELETE CASCADE
			)`,
			"export_logs": `CREATE TABLE IF NOT EXISTS export_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				username TEXT NOT NULL,
				connection_name TEXT NOT NULL DEFAULT '',
				sql_statement TEXT NOT NULL DEFAULT '',
				row_count INTEGER NOT NULL DEFAULT 0,
				file_type TEXT NOT NULL DEFAULT '',
				file_name TEXT NOT NULL DEFAULT '',
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY(user_id) REFERENCES users(id)
			)`,
		}
	}
}

// buildInitSQL 生成完整的手动初始化 SQL 脚本，用于数据库用户无 DDL 权限时手动执行。
func buildInitSQL(dbType string) string {
	tables := getTableDDLs(dbType)
	var sb strings.Builder

	sb.WriteString("-- SQL Panel initialization script\n")
	sb.WriteString(fmt.Sprintf("-- Database type: %s\n\n", dbType))

	for name, ddl := range tables {
		sb.WriteString(fmt.Sprintf("-- Table: %s\n", name))
		sb.WriteString(ddl)
		sb.WriteString(";\n\n")
	}

	sb.WriteString("-- Default admin user (password: admin123)\n")
	switch dbType {
	case "mysql":
		sb.WriteString("INSERT IGNORE INTO users (username, password, role) VALUES ('admin', '$2a$10$1t59Wgfp8bfaFEwKBn0ziOWraWrhS5MG1Dmf3/HEXkPWmVrVaXvlq', 'admin');\n")
	case "postgres":
		sb.WriteString("INSERT INTO users (username, password, role) VALUES ('admin', '$2a$10$1t59Wgfp8bfaFEwKBn0ziOWraWrhS5MG1Dmf3/HEXkPWmVrVaXvlq', 'admin') ON CONFLICT (username) DO NOTHING;\n")
	default:
		sb.WriteString("INSERT OR IGNORE INTO users (username, password, role) VALUES ('admin', '$2a$10$1t59Wgfp8bfaFEwKBn0ziOWraWrhS5MG1Dmf3/HEXkPWmVrVaXvlq', 'admin');\n")
	}

	return sb.String()
}
// Package models 定义系统核心数据结构，包括用户、数据库连接等模型。
package models

// DBType 数据库类型枚举。
type DBType string

const (
	PostgreSQL DBType = "postgres"
	MySQL      DBType = "mysql"
	SQLite     DBType = "sqlite"
	SQLServer  DBType = "sqlserver"
)

// User 系统用户信息。
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"` // json:"-" 确保密码不会序列化到 JSON 响应中
	Role     string `json:"role"`
}

// Connection 数据库连接配置，包含连接目标数据库所需的所有参数。
type Connection struct {
	ID           string `json:"id"`
	CreatorID    int    `json:"creatorId"`    // 创建者用户 ID
	Name         string `json:"name"`         // 连接别名
	Type         DBType `json:"type"`         // 数据库类型
	Host         string `json:"host"`         // 主机地址（SQLite 时为文件路径）
	Port         int    `json:"port"`         // 端口号
	User         string `json:"user"`         // 数据库用户名
	Password     string `json:"password"`     // 数据库密码
	DatabaseName string `json:"database_name"` // 数据库名称
}
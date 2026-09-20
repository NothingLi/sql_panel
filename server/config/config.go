// Package config 提供应用级配置的统一入口。
// 所有配置项优先从环境变量读取，未设置时使用默认值。
package config

import (
	"os"
	"strconv"
	"time"
)

// JWTSecret 返回 JWT 签名/验证密钥。
// 生产环境应通过环境变量 JWT_SECRET 设置。
func JWTSecret() []byte {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		return []byte(secret)
	}
	return []byte("your_secret_key_sql_panel")
}

// QueryTimeout 返回 SELECT 查询超时时间，默认 300 秒（5 分钟）。
// 可通过环境变量 QUERY_TIMEOUT_SEC 覆盖。
func QueryTimeout() time.Duration {
	if s := os.Getenv("QUERY_TIMEOUT_SEC"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 300 * time.Second
}

// ExecTimeout 返回 INSERT/UPDATE/DELETE/DDL 执行超时时间，默认 300 秒（5 分钟）。
// 可通过环境变量 EXEC_TIMEOUT_SEC 覆盖。
func ExecTimeout() time.Duration {
	if s := os.Getenv("EXEC_TIMEOUT_SEC"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 300 * time.Second
}

// DDLLockTimeout 返回等待 DDL 表级锁的超时时间，默认 60 秒。
// 可通过环境变量 DDL_LOCK_TIMEOUT_SEC 覆盖。
func DDLLockTimeout() time.Duration {
	if s := os.Getenv("DDL_LOCK_TIMEOUT_SEC"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 60 * time.Second
}

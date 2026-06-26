package db

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"sql_panel/server/config"
	"strings"
	"sync"
	"time"
)

// TableLocker 表级互斥锁管理器，防止并发 DDL 操作冲突。
// 每个表一个 sync.Mutex，DDL 执行期间阻止其他会话对同一张表执行 DDL。
type TableLocker struct {
	mu    sync.Mutex
	locks map[string]*tableLock
}

// tableLock 单个表的锁，带引用计数和最后使用时间。
type tableLock struct {
	mu      sync.Mutex
	refs    int           // 引用计数
	lastUse time.Time     // 最后一次使用时间
}

// DefaultTableLocker 全局表锁管理器实例。
var DefaultTableLocker = &TableLocker{
	locks: make(map[string]*tableLock),
}

// Lock 获取指定表名的互斥锁。阻塞等待直到锁可用。
func (tl *TableLocker) Lock(tableName string) {
	tl.mu.Lock()
	tlck, ok := tl.locks[tableName]
	if !ok {
		tlck = &tableLock{lastUse: time.Now()}
		tl.locks[tableName] = tlck
	}
	tlck.refs++
	tl.mu.Unlock()

	tlck.mu.Lock()
}

// Unlock 释放指定表名的互斥锁。
func (tl *TableLocker) Unlock(tableName string) {
	tl.mu.Lock()
	tlck, ok := tl.locks[tableName]
	if !ok {
		tl.mu.Unlock()
		return
	}
	tlck.refs--
	tlck.lastUse = time.Now()
	if tlck.refs <= 0 {
		delete(tl.locks, tableName)
	}
	tl.mu.Unlock()

	tlck.mu.Unlock()
}

// TryLock 尝试获取锁，如果无法立即获取则返回 false。
func (tl *TableLocker) TryLock(tableName string) bool {
	tl.mu.Lock()
	tlck, ok := tl.locks[tableName]
	if !ok {
		tlck = &tableLock{lastUse: time.Now()}
		tl.locks[tableName] = tlck
	}
	tlck.refs++
	tl.mu.Unlock()

	return tlck.mu.TryLock()
}

// LockWithTimeout 带超时的锁获取。返回是否成功获取。
func (tl *TableLocker) LockWithTimeout(tableName string, timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		tl.Lock(tableName)
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("acquire lock timeout for table %s after %v", tableName, timeout)
	}
}

// ddlTablePatterns SQL 语句模式，用于提取 DDL 操作涉及的表名。
var ddlTablePatterns = []struct {
	pattern *regexp.Regexp
	group   int // 表名所在的捕获组索引
}{
	// CREATE TABLE [IF NOT EXISTS] tableName
	{regexp.MustCompile(`(?i)^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`" + `?(\w+)` + "`" + `?`), 1},
	// ALTER TABLE [IF EXISTS] tableName
	{regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?` + "`" + `?(\w+)` + "`" + `?`), 1},
	// DROP TABLE [IF EXISTS] tableName
	{regexp.MustCompile(`(?i)^DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?` + "`" + `?(\w+)` + "`" + `?`), 1},
	// TRUNCATE [TABLE] tableName
	{regexp.MustCompile(`(?i)^TRUNCATE\s+(?:TABLE\s+)?` + "`" + `?(\w+)` + "`" + `?`), 1},
	// CREATE INDEX [IF NOT EXISTS] indexName ON tableName
	{regexp.MustCompile(`(?i)^CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`" + `?\w+` + "`" + `?\s+ON\s+` + "`" + `?(\w+)` + "`" + `?`), 1},
	// DROP INDEX [IF EXISTS] indexName ON tableName
	{regexp.MustCompile(`(?i)^DROP\s+INDEX\s+(?:IF\s+EXISTS\s+)?` + "`" + `?\w+` + "`" + `?\s+ON\s+` + "`" + `?(\w+)` + "`" + `?`), 1},
	// RENAME TABLE oldName TO newName → 只锁旧表名
	{regexp.MustCompile(`(?i)^RENAME\s+TABLE\s+` + "`" + `?(\w+)` + "`" + `?\s+TO`), 1},
}

// isDDL 判断 SQL 语句是否为 DDL 操作。
func isDDL(sql string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	for _, prefix := range []string{
		"CREATE TABLE", "ALTER TABLE", "DROP TABLE", "TRUNCATE",
		"CREATE INDEX", "CREATE UNIQUE INDEX", "DROP INDEX", "RENAME TABLE",
	} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// extractTableNames 从 SQL 语句提取涉及的表名。
func extractTableNames(sql string) []string {
	trimmed := strings.TrimSpace(sql)
	var names []string
	seen := make(map[string]bool)

	for _, p := range ddlTablePatterns {
		matches := p.pattern.FindStringSubmatch(trimmed)
		if matches != nil && len(matches) > p.group {
			name := strings.ToLower(strings.Trim(matches[p.group], "`\"[]"))
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	// RENAME TABLE 特殊处理：还需要锁新表名
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "RENAME TABLE") {
		renameRe := regexp.MustCompile(`(?i)^RENAME\s+TABLE\s+` + "`" + `?\w+` + "`" + `?\s+TO\s+` + "`" + `?(\w+)` + "`" + `?`)
		matches := renameRe.FindStringSubmatch(trimmed)
		if matches != nil && len(matches) > 1 {
			name := strings.ToLower(strings.Trim(matches[1], "`\"[]"))
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	return names
}

// ExecuteWithDDLLock 在持有表级锁的情况下执行 DDL 操作。
// 自动识别 SQL 中涉及的表名，加锁后执行 fn，执行完成后释放锁。
// 锁超时默认 60 秒。
func (tl *TableLocker) ExecuteWithDDLLock(sql string, fn func() error) error {
	if !isDDL(sql) {
		return fn()
	}

	tableNames := extractTableNames(sql)
	if len(tableNames) == 0 {
		log.Printf("[DDL Lock] unable to parse table name from: %s", sql)
		return fn()
	}

	// 按字母序加锁，避免死锁（所有并发请求按相同顺序获取锁）
	sort.Strings(tableNames)

	log.Printf("[DDL Lock] acquiring locks for tables: %v, sql: %s", tableNames, sql)
	timeout := config.DDLLockTimeout()
	for _, name := range tableNames {
		if err := tl.LockWithTimeout(name, timeout); err != nil {
			// 释放已获取的锁
			for _, n := range tableNames {
				if n == name {
					break
				}
				tl.Unlock(n)
			}
			return err
		}
	}

	defer func() {
		for i := len(tableNames) - 1; i >= 0; i-- {
			tl.Unlock(tableNames[i])
		}
		log.Printf("[DDL Lock] released locks for tables: %v", tableNames)
	}()

	return fn()
}


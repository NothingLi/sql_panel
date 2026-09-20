// Package handlers 提供 SQL 查询执行和数据库操作相关的 HTTP 处理函数。
package handlers

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sql_panel/server/config"
	"sql_panel/server/db"
	"sql_panel/server/models"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// QueryRequest SQL 查询请求体。
type QueryRequest struct {
	SQL          string `json:"sql"`
	ConnectionId string `json:"connectionId"`
	Timeout      int    `json:"timeout"` // 超时秒数，0 表示使用默认值
}

// getQueryTimeout 返回请求指定的超时时间，若未指定则使用全局配置。
func getQueryTimeout(reqTimeout int) time.Duration {
	if reqTimeout > 0 {
		return time.Duration(reqTimeout) * time.Second
	}
	return config.QueryTimeout()
}

// getExecTimeout 返回请求指定的超时时间，若未指定则使用全局配置。
func getExecTimeout(reqTimeout int) time.Duration {
	if reqTimeout > 0 {
		return time.Duration(reqTimeout) * time.Second
	}
	return config.ExecTimeout()
}

// QueryResponse SQL 查询响应体，包含列名和数据行。
type QueryResponse struct {
	Columns []string `json:"columns"`
	Rows    []Row    `json:"rows"`
}

// Row 一行查询结果，key 为列名，value 为单元格值（字符串或 null）。
type Row map[string]interface{}

// ExecuteQuery 统一 SQL 执行入口，根据 SQL 类型自动分发：
//   - SELECT/SHOW/DESCRIBE/EXPLAIN → 查询模式，返回列名 + 行数据
//   - BEGIN/COMMIT/ROLLBACK → 事务控制命令
//   - INSERT/UPDATE/DELETE 等 → 非查询模式，返回受影响行数
//
// 支持在活跃事务中执行。
func ExecuteQuery(c *gin.Context) {
	userId := c.MustGet("userId").(int)

	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	id := req.ConnectionId
	sqlQuery := strings.TrimSpace(req.SQL)

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	// 安全检查：拒绝 SYSTEM 前缀
	upperSQL := strings.ToUpper(sqlQuery)
	if strings.HasPrefix(upperSQL, "SYSTEM") {
		logError(c, http.StatusBadRequest, "System commands are not allowed")
		return
	}

	// 事务控制命令
	if upperSQL == "BEGIN" || upperSQL == "COMMIT" || upperSQL == "ROLLBACK" {
		handleTransactionCommand(c, userId, connInfo, upperSQL)
		return
	}

	// 判断是否为查询类 SQL（返回结果集）
	isQuery := strings.HasPrefix(upperSQL, "SELECT") ||
		strings.HasPrefix(upperSQL, "SHOW") ||
		strings.HasPrefix(upperSQL, "DESCRIBE") ||
		strings.HasPrefix(upperSQL, "EXPLAIN") ||
		strings.HasPrefix(upperSQL, "PRAGMA") ||
		strings.HasPrefix(upperSQL, "WITH")

	if isQuery {
		queryTimeout := getQueryTimeout(req.Timeout)
		if activeTx := db.Pool.GetTx(connInfo.ID, userId); activeTx != nil {
			executeWithTx(c, activeTx, sqlQuery, queryTimeout)
			return
		}
		dbConn, connErr := db.Pool.GetConnection(connInfo)
		if connErr != nil {
			logError(c, http.StatusInternalServerError, connErr.Error())
			return
		}
		executeQuery(c, dbConn, sqlQuery, queryTimeout)
		return
	}

	// 非查询 SQL（INSERT/UPDATE/DELETE/DDL 等），支持多条语句
	statements := splitSQLStatements(sqlQuery)
	var totalRowsAffected int64
	var lastInsertId int64

	execStmts := func() error {
		hasActiveTx := db.Pool.GetTx(connInfo.ID, userId) != nil
		ctx, cancel := context.WithTimeout(context.Background(), getExecTimeout(req.Timeout))
		defer cancel()
		for _, stmt := range statements {
			var result sql.Result
			if hasActiveTx {
				result, err = db.Pool.GetTx(connInfo.ID, userId).ExecContext(ctx, stmt)
			} else {
				dbConn, connErr := db.Pool.GetConnection(connInfo)
				if connErr != nil {
					return connErr
				}
				result, err = dbConn.ExecContext(ctx, stmt)
			}
			if err != nil {
				return err
			}
			rows, err := result.RowsAffected()
			if err != nil {
				log.Printf("WARNING: RowsAffected failed: %v", err)
			}
			totalRowsAffected += rows
			if lid, err2 := result.LastInsertId(); err2 == nil && lid > 0 {
				lastInsertId = lid
			}
		}
		return nil
	}

	// DDL 操作需要表级锁保护，防止并发冲突
	ddlErr := db.DefaultTableLocker.ExecuteWithDDLLock(sqlQuery, execStmts)
	if ddlErr != nil {
		logError(c, http.StatusInternalServerError, "Execute error: "+ddlErr.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rowsAffected": totalRowsAffected,
		"lastInsertId": lastInsertId,
	})
}

// handleTransactionCommand 处理事务控制命令（BEGIN/COMMIT/ROLLBACK）。
func handleTransactionCommand(c *gin.Context, userId int, connInfo models.Connection, action string) {
	switch action {
	case "BEGIN":
		_, err := db.Pool.BeginTx(connInfo, userId)
		if err != nil {
			logError(c, http.StatusInternalServerError, "Failed to begin transaction: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Transaction started", "transactionActive": true})
	case "COMMIT":
		if err := db.Pool.CommitTx(connInfo.ID, userId); err != nil {
			logError(c, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Transaction committed", "transactionActive": false})
	case "ROLLBACK":
		if err := db.Pool.RollbackTx(connInfo.ID, userId); err != nil {
			logError(c, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Transaction rolled back", "transactionActive": false})
	}
}

// executeQuery 执行 SQL 查询并将结果序列化为 JSON 返回。
func executeQuery(c *gin.Context, dbConn *sql.DB, sql string, timeout time.Duration) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := dbConn.QueryContext(ctx, sql)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var result []Row
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(Row)
		for i, col := range columns {
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	if result == nil {
		result = make([]Row, 0)
	}

	duration := time.Since(start).String()

	c.JSON(http.StatusOK, gin.H{
		"columns":  columns,
		"rows":     result,
		"duration": duration,
	})
}

// executeWithTx 在活跃事务中执行查询。
func executeWithTx(c *gin.Context, tx *sql.Tx, sqlStatement string, timeout time.Duration) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := tx.QueryContext(ctx, sqlStatement)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var result []Row
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(Row)
		for i, col := range columns {
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	if result == nil {
		result = make([]Row, 0)
	}

	duration := time.Since(start).String()

	c.JSON(http.StatusOK, gin.H{
		"columns":  columns,
		"rows":     result,
		"duration": duration,
	})
}

// TableSchema 返回给前端的表结构信息，包含表名和列列表。
type TableSchema struct {
	Name    string       `json:"name"`
	Columns []ColumnInfo `json:"columns"`
}

// ColumnInfo 列信息，包含名称、类型和注释。
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

// GetTables 获取指定数据库中所有用户表的表名和列信息。
// 返回格式：[{name: "users", columns: ["id (INTEGER)", "name (TEXT)"]}, ...]
func GetTables(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var tableQuery string
	switch connInfo.Type {
	case models.SQLite:
		tableQuery = "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
	case models.MySQL:
		tableQuery = "SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY TABLE_NAME"
	case models.PostgreSQL:
		tableQuery = `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema NOT IN ('pg_catalog', 'information_schema') AND table_name NOT LIKE 'pg_%'
			ORDER BY table_name`
	default:
		logError(c, http.StatusNotImplemented, "Not implemented for "+string(connInfo.Type))
		return
	}

	rows, err := dbConn.Query(tableQuery)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tableNames = append(tableNames, name)
		}
	}

	// 为每个表获取列信息
	schema := make([]TableSchema, 0, len(tableNames))
	for _, tableName := range tableNames {
		columns := getTableColumns(dbConn, connInfo.Type, tableName)
		schema = append(schema, TableSchema{
			Name:    tableName,
			Columns: columns,
		})
	}

	c.JSON(http.StatusOK, schema)
}

// getTableColumns 获取指定表的列信息，包含列名、类型和注释。
// tableName 来自系统表查询，非用户直接输入，但仍做转义防止注入。
func getTableColumns(dbConn *sql.DB, dbType models.DBType, tableName string) []ColumnInfo {
	switch dbType {
	case models.SQLite:
		// PRAGMA 不支持参数化查询，通过转义单引号防止注入
		safeName := strings.ReplaceAll(tableName, "'", "''")
		colQuery := fmt.Sprintf("PRAGMA table_info('%s')", safeName)
		rows, err := dbConn.Query(colQuery)
		if err != nil {
			return []ColumnInfo{}
		}
		defer rows.Close()

		var columns []ColumnInfo
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull int
			var dfltValue, pk interface{}
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
				continue
			}
			columns = append(columns, ColumnInfo{Name: name, Type: colType})
		}
		return columns

	case models.MySQL:
		colQuery := `
			SELECT column_name, data_type, IFNULL(column_comment, '')
			FROM information_schema.columns
			WHERE table_name = ? AND table_schema = DATABASE()
			ORDER BY ordinal_position`
		rows, err := dbConn.Query(colQuery, tableName)
		if err != nil {
			return []ColumnInfo{}
		}
		defer rows.Close()

		var columns []ColumnInfo
		for rows.Next() {
			var name, dataType, comment string
			if err := rows.Scan(&name, &dataType, &comment); err != nil {
				continue
			}
			columns = append(columns, ColumnInfo{Name: name, Type: dataType, Comment: comment})
		}
		return columns

	case models.PostgreSQL:
		colQuery := `
			SELECT c.column_name, c.data_type, 
			       COALESCE(pg_catalog.col_description(c.table_name::regclass::oid, c.ordinal_position), '')
			FROM information_schema.columns c
			WHERE c.table_name = $1
			ORDER BY c.ordinal_position`
		rows, err := dbConn.Query(colQuery, tableName)
		if err != nil {
			return []ColumnInfo{}
		}
		defer rows.Close()

		var columns []ColumnInfo
		for rows.Next() {
			var name, dataType, comment string
			if err := rows.Scan(&name, &dataType, &comment); err != nil {
				continue
			}
			columns = append(columns, ColumnInfo{Name: name, Type: dataType, Comment: comment})
		}
		return columns
	}

	return []ColumnInfo{}
}

// GetTableColumns 获取指定表的列信息（列名和数据类型）。
func GetTableColumns(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")
	tableName := c.Param("table")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var colQuery string
	switch connInfo.Type {
	case models.SQLite:
		colQuery = fmt.Sprintf("PRAGMA table_info(%s)", tableName)
		rows, err := dbConn.Query(colQuery)
		if err != nil {
			logError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		type ColumnInfo struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			NotNull bool   `json:"notnull"`
			PK      int    `json:"pk"`
		}

		var columns []ColumnInfo
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull int
			var dfltValue, pk interface{}

			if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
				continue
			}

			columns = append(columns, ColumnInfo{
				Name:    name,
				Type:    colType,
				NotNull: notNull == 1,
				PK:      cid,
			})
		}

		c.JSON(http.StatusOK, gin.H{"columns": columns})
		return

	case models.MySQL, models.PostgreSQL:
		colQuery = fmt.Sprintf(`
			SELECT column_name, data_type, is_nullable, column_default, column_comment
			FROM information_schema.columns
			WHERE table_name = '%s'`, tableName)

		if connInfo.Type == models.MySQL {
			colQuery += " AND table_schema = DATABASE()"
		}

		rows, err := dbConn.Query(colQuery)
		if err != nil {
			logError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		type ColumnInfo struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			NotNull bool   `json:"notnull"`
			PK      int    `json:"pk"`
			Comment string `json:"comment"`
		}

		var columns []ColumnInfo
		for rows.Next() {
			var name, colType, nullable string
			var defaultValue, comment *string
			if err := rows.Scan(&name, &colType, &nullable, &defaultValue, &comment); err != nil {
				continue
			}
			colComment := ""
			if comment != nil {
				colComment = *comment
			}
			columns = append(columns, ColumnInfo{
				Name:    name,
				Type:    colType,
				NotNull: nullable == "NO",
				Comment: colComment,
			})
		}

		c.JSON(http.StatusOK, gin.H{"columns": columns})
		return

	default:
		logError(c, http.StatusNotImplemented, "Not implemented for "+string(connInfo.Type))
		return
	}
}

// GetTableDDL 获取指定表的 DDL（Data Definition Language）语句。
func GetTableDDL(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")
	tableName := c.Param("table")

	// 1. 查找连接
	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	// 2. 建立连接
	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var ddl string
	switch connInfo.Type {
	case models.SQLite:
		// SQLite 直接从 sqlite_master 表中读取 sql 字段
		err = dbConn.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&ddl)
	case models.MySQL:
		// MySQL 使用 SHOW CREATE TABLE，用反引号包裹表名防止注入
		safeName := "`" + strings.ReplaceAll(tableName, "`", "``") + "`"
		var dummy string
		err = dbConn.QueryRow(fmt.Sprintf("SHOW CREATE TABLE %s", safeName)).Scan(&dummy, &ddl)
	case models.PostgreSQL:
		// Postgres 没有简单的 SHOW CREATE TABLE，这里我们通过查询元数据生成一个基础版本
		// 生产环境下通常会使用更复杂的存储过程或工具函数
		query := `
			SELECT 
				'CREATE TABLE ' || table_name || ' (' || 
				string_agg(column_name || ' ' || data_type, ', ') || 
				');'
			FROM information_schema.columns 
			WHERE table_name = ?
			GROUP BY table_name`
		err = dbConn.QueryRow(query, tableName).Scan(&ddl)
	default:
		logError(c, http.StatusNotImplemented, "DDL view not implemented for "+string(connInfo.Type))
		return
	}

	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to fetch DDL: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"ddl": ddl})
}

// ExportCSV 将 SQL 查询结果导出为 CSV 文件下载。
func ExportCSV(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), getQueryTimeout(req.Timeout))
	defer cancel()

	rows, err := dbConn.QueryContext(ctx, req.SQL)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 设置响应头为 CSV 下载
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Content-Disposition", "attachment; filename=export.csv")

	writer := csv.NewWriter(c.Writer)
	// 写入 BOM 以确保 Excel 正确识别 UTF-8 编码
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	// 写入表头
	if err := writer.Write(columns); err != nil {
		return
	}

	// 写入数据行
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		record := make([]string, len(columns))
		for i, val := range values {
			b, ok := val.([]byte)
			if ok {
				record[i] = string(b)
			} else if val == nil {
				record[i] = ""
			} else {
				record[i] = fmt.Sprintf("%v", val)
			}
		}
		writer.Write(record)
	}

	writer.Flush()
}

// ExportJSON 将 SQL 查询结果导出为 JSON 文件下载。
func ExportJSON(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), getQueryTimeout(req.Timeout))
	defer cancel()

	rows, err := dbConn.QueryContext(ctx, req.SQL)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var result []Row
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(Row)
		for i, col := range columns {
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	// 序列化为 JSON 并下载
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to marshal JSON")
		return
	}

	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.Header().Set("Content-Disposition", "attachment; filename=export.json")
	c.Writer.Write(jsonData)
}

// GetDatabases 获取指定数据库服务器上的所有数据库名称。
func GetDatabases(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		fmt.Println("Error:", err)
		logError(c, http.StatusInternalServerError, "Connection not found or permission denied")
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 根据不同数据库类型获取数据库列表
	var dbQuery string
	switch connInfo.Type {
	case models.SQLite:
		// SQLite 单文件，只返回配置的数据库名
		c.JSON(http.StatusOK, gin.H{"databases": []string{connInfo.DatabaseName}})
		return
	case models.MySQL:
		dbQuery = "SHOW DATABASES"
	case models.PostgreSQL:
		dbQuery = "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"
	default:
		logError(c, http.StatusNotImplemented, "Not implemented for "+string(connInfo.Type))
		return
	}

	rows, err := dbConn.Query(dbQuery)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			databases = append(databases, name)
		}
	}

	c.JSON(http.StatusOK, gin.H{"databases": databases})
}

// TransactionRequest 事务操作请求体。
type TransactionRequest struct {
	Action string `json:"action"` // begin / commit / rollback
}

// HandleTransaction 处理事务操作（开始、提交、回滚）。
func HandleTransaction(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	var req TransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	switch req.Action {
	case "begin":
		// 开启新事务
		_, err := db.Pool.BeginTx(connInfo, userId)
		if err != nil {
			logError(c, http.StatusInternalServerError, "Failed to begin transaction: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Transaction started"})
	case "commit":
		// 提交当前事务
		if err := db.Pool.CommitTx(connInfo.ID, userId); err != nil {
			logError(c, http.StatusInternalServerError, "Failed to commit: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Transaction committed"})
	case "rollback":
		// 回滚当前事务
		if err := db.Pool.RollbackTx(connInfo.ID, userId); err != nil {
			logError(c, http.StatusInternalServerError, "Failed to rollback: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Transaction rolled back"})
	default:
		logError(c, http.StatusBadRequest, "Invalid action. Use: begin, commit, rollback")
	}
}

// splitSQLStatements 将多条 SQL 语句按分号拆分为独立语句。
// 能正确处理单引号字符串、双引号字符串、单行注释(--)和多行注释(/* */)中的分号。
func splitSQLStatements(sqlText string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false

	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// 处理行注释结束（换行符）
		if inLineComment {
			current.WriteRune(ch)
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}

		// 处理块注释
		if inBlockComment {
			current.WriteRune(ch)
			if ch == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				current.WriteRune(runes[i+1])
				i++
				inBlockComment = false
			}
			continue
		}

		// 处理单引号字符串
		if inSingleQuote {
			current.WriteRune(ch)
			if ch == '\\' && i+1 < len(runes) {
				current.WriteRune(runes[i+1])
				i++
			} else if ch == '\'' {
				inSingleQuote = false
			}
			continue
		}

		// 处理双引号字符串
		if inDoubleQuote {
			current.WriteRune(ch)
			if ch == '\\' && i+1 < len(runes) {
				current.WriteRune(runes[i+1])
				i++
			} else if ch == '"' {
				inDoubleQuote = false
			}
			continue
		}

		// 检测注释和字符串开始
		switch ch {
		case '\'':
			inSingleQuote = true
			current.WriteRune(ch)
		case '"':
			inDoubleQuote = true
			current.WriteRune(ch)
		case '-':
			if i+1 < len(runes) && runes[i+1] == '-' {
				inLineComment = true
				current.WriteRune(ch)
				current.WriteRune(runes[i+1])
				i++
			} else {
				current.WriteRune(ch)
			}
		case '/':
			if i+1 < len(runes) && runes[i+1] == '*' {
				inBlockComment = true
				current.WriteRune(ch)
				current.WriteRune(runes[i+1])
				i++
			} else {
				current.WriteRune(ch)
			}
		case ';':
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}

	// 最后一条语句（可能没有分号结尾）
	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// ExecuteNonQuery 执行非查询 SQL（INSERT/UPDATE/DELETE 等）。
// 支持多条语句（分号分隔），逐条执行并汇总影响行数。
func ExecuteNonQuery(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	sqlQuery := strings.TrimSpace(req.SQL)

	// 安全检查：拒绝 SYSTEM 前缀的敏感 SQL
	parts := strings.Fields(sqlQuery)
	if len(parts) > 0 && strings.ToUpper(parts[0]) == "SYSTEM" {
		logError(c, http.StatusBadRequest, "System commands are not allowed")
		return
	}

	// 支持下划线分隔的 SYSTEM_VACUUM、SYSTEM_PRAGMA 等
	upperSQL := strings.ToUpper(sqlQuery)
	if strings.HasPrefix(upperSQL, "SYSTEM_") {
		logError(c, http.StatusBadRequest, "System commands are not allowed")
		return
	}

	// 拆分多条语句，逐条执行
	statements := splitSQLStatements(sqlQuery)

	var totalRowsAffected int64
	var lastInsertId int64

	execStmts := func() error {
		hasActiveTx := db.Pool.GetTx(connInfo.ID, userId) != nil
		ctx, cancel := context.WithTimeout(context.Background(), getExecTimeout(req.Timeout))
		defer cancel()
		for _, stmt := range statements {
			var result sql.Result
			if hasActiveTx {
				result, err = db.Pool.GetTx(connInfo.ID, userId).ExecContext(ctx, stmt)
			} else {
				dbConn, connErr := db.Pool.GetConnection(connInfo)
				if connErr != nil {
					return connErr
				}
				result, err = dbConn.ExecContext(ctx, stmt)
			}
			if err != nil {
				return err
			}
			rows, err := result.RowsAffected()
			if err != nil {
				log.Printf("WARNING: RowsAffected failed: %v", err)
			}
			totalRowsAffected += rows
			if lid, err2 := result.LastInsertId(); err2 == nil && lid > 0 {
				lastInsertId = lid
			}
		}
		return nil
	}

	ddlErr := db.DefaultTableLocker.ExecuteWithDDLLock(sqlQuery, execStmts)
	if ddlErr != nil {
		logError(c, http.StatusInternalServerError, ddlErr.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rowsAffected": totalRowsAffected,
		"lastInsertId": lastInsertId,
	})
}

// FormatSQL SQL 格式化请求体。
type FormatSQL struct {
	SQL string `json:"sql" binding:"required"`
}

// FormatSQLHandler 简单的 SQL 格式化：关键字大写 + 基本缩进。
func FormatSQLHandler(c *gin.Context) {
	var req FormatSQL
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, "Missing SQL")
		return
	}

	formatted := formatSQLFunc(req.SQL)
	c.JSON(http.StatusOK, gin.H{"result": formatted})
}

// formatSQLFunc 简单格式化 SQL：关键字大写、换行处理。
func formatSQLFunc(raw string) string {
	raw = strings.TrimSpace(raw)

	// 常见 SQL 关键字列表
	keywords := []string{
		"SELECT", "FROM", "WHERE", "AND", "OR", "NOT", "IN", "EXISTS",
		"INSERT", "INTO", "VALUES", "UPDATE", "SET",
		"DELETE", "CREATE", "TABLE", "ALTER", "DROP", "INDEX",
		"JOIN", "LEFT", "RIGHT", "INNER", "OUTER", "ON",
		"ORDER", "BY", "GROUP", "HAVING", "LIMIT", "OFFSET",
		"ASC", "DESC", "AS", "DISTINCT", "UNION", "ALL",
		"IS", "NULL", "LIKE", "BETWEEN", "CASE", "WHEN",
		"THEN", "ELSE", "END", "COUNT", "SUM", "AVG",
		"MIN", "MAX", "BEGIN", "COMMIT", "ROLLBACK",
	}

	// 分词并大写关键字
	words := strings.Fields(raw)
	for i, w := range words {
		upperW := strings.ToUpper(w)
		for _, kw := range keywords {
			if upperW == kw {
				words[i] = upperW
				break
			}
		}
	}

	result := strings.Join(words, " ")

	// 在主要子句前添加换行
	lineClauses := []string{"SELECT", "FROM", "WHERE", "AND", "ORDER BY", "GROUP BY", "LIMIT", "INSERT INTO", "VALUES", "SET", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "ON"}
	for _, clause := range lineClauses {
		result = strings.ReplaceAll(result, " "+clause+" ", "\n"+clause+" ")
	}

	return strings.TrimSpace(result)
}

// GetTransactionStatus 查询当前连接的事务状态（是否有活跃事务）。
func GetTransactionStatus(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	tx := db.Pool.GetTx(id, userId)
	c.JSON(http.StatusOK, gin.H{
		"active": tx != nil,
	})
}

// ExecuteRawQuery 执行任意原始 SQL 并返回 JSON 结果。
// 根据 SQL 类型自动选择查询或执行方式。
func ExecuteRawQuery(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	sqlQuery := strings.TrimSpace(req.SQL)
	if sqlQuery == "" {
		logError(c, http.StatusBadRequest, "SQL query cannot be empty")
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	executeQuery(c, dbConn, sqlQuery, getQueryTimeout(req.Timeout))
}

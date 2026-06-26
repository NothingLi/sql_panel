// Package handlers 提供系统级数据管理相关的 HTTP 处理函数，如查询日志。
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sql_panel/server/db"
	"time"

	"github.com/gin-gonic/gin"
)

// QueryLog 查询日志记录。
type QueryLog struct {
	ID             int    `json:"id"`
	UserID         int    `json:"userId"`
	Username       string `json:"username"`
	ConnectionName string `json:"connectionName"`
	SQLStatement   string `json:"sqlStatement"`
	RowCount       int    `json:"rowCount"`
	ExecutionTime  int    `json:"executionTime"`
	Status         string `json:"status"`
	ErrorMessage   string `json:"errorMessage"`
	CreatedAt      string `json:"createdAt"`
}

// SaveQueryLogRequest 保存查询日志的请求体。
type SaveQueryLogRequest struct {
	ConnectionName string `json:"connectionName"`
	SQLStatement   string `json:"sqlStatement"`
	RowCount       int    `json:"rowCount"`
	ExecutionTime  int    `json:"executionTime"`
	Status         string `json:"status"`
	ErrorMessage   string `json:"errorMessage"`
}

// SaveQueryLog 保存一条查询日志到系统数据库。
func SaveQueryLog(c *gin.Context) {
	userId := c.MustGet("userId").(int)

	var username string
	db.DB.QueryRow("SELECT username FROM users WHERE id = ?", userId).Scan(&username)

	var req SaveQueryLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	_, err := db.DB.Exec(
		`INSERT INTO query_logs (user_id, username, connection_name, sql_statement, row_count, execution_time, status, error_message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userId, username, req.ConnectionName, req.SQLStatement, req.RowCount, req.ExecutionTime, req.Status, req.ErrorMessage, time.Now().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to save query log: " + err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Query log saved"})
}

// GetQueryLogs 获取当前用户的查询日志（最近 100 条）。
// admin 角色可查看所有用户的日志。
func GetQueryLogs(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	role := c.MustGet("role").(string)

	var rows *sql.Rows
	var err error

	if role == "admin" {
		rows, err = db.DB.Query(
			`SELECT id, user_id, username, connection_name, sql_statement, row_count, execution_time, status, COALESCE(error_message, ''), created_at
			 FROM query_logs ORDER BY created_at DESC LIMIT 100`)
	} else {
		rows, err = db.DB.Query(
			`SELECT id, user_id, username, connection_name, sql_statement, row_count, execution_time, status, COALESCE(error_message, ''), created_at
			 FROM query_logs WHERE user_id = ? ORDER BY created_at DESC LIMIT 100`, userId)
	}

	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to query query logs: " + err.Error())
		return
	}
	defer rows.Close()

	var logs []QueryLog
	for rows.Next() {
		var log QueryLog
		if err := rows.Scan(&log.ID, &log.UserID, &log.Username, &log.ConnectionName, &log.SQLStatement, &log.RowCount, &log.ExecutionTime, &log.Status, &log.ErrorMessage, &log.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, log)
	}

	if logs == nil {
		logs = []QueryLog{}
	}

	c.JSON(http.StatusOK, logs)
}

// SystemBackup 系统备份数据结构，包含所有需要导出的数据表。
type SystemBackup struct {
	Users             []map[string]interface{} `json:"users"`
	Connections       []map[string]interface{} `json:"connections"`
	SharedConnections []map[string]interface{} `json:"shared_connections"`
	// QueryLogs         []map[string]interface{} `json:"query_logs"`
	ExportLogs        []map[string]interface{} `json:"export_logs"`
}

// ExportSystemData 导出系统所有数据作为备份（仅 admin）。
func ExportSystemData(c *gin.Context) {
	role := c.MustGet("role").(string)
	if role != "admin" {
		logError(c, http.StatusForbidden, "Only admin can export system data")
		return
	}

	tables := []string{"users", "connections", "shared_connections",  "export_logs"}
	backup := SystemBackup{}

	for _, table := range tables {
		rows, err := db.DB.Query("SELECT * FROM " + table)
		if err != nil {
			logError(c, http.StatusInternalServerError, "Failed to query " + table + ": " + err.Error())
			return
		}

		columns, _ := rows.Columns()
		var result []map[string]interface{}

		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range columns {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				continue
			}
			row := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					row[col] = string(b)
				} else {
					row[col] = val
				}
			}
			result = append(result, row)
		}
		rows.Close()

		if result == nil {
			result = []map[string]interface{}{}
		}

		switch table {
		case "users":
			backup.Users = result
		case "connections":
			backup.Connections = result
		case "shared_connections":
			backup.SharedConnections = result
		// case "query_logs":
		// 	backup.QueryLogs = result
		case "export_logs":
			backup.ExportLogs = result
		}
	}

	c.JSON(http.StatusOK, backup)
}

// ImportSystemData 从备份文件导入系统数据（仅 admin）。
func ImportSystemData(c *gin.Context) {
	role := c.MustGet("role").(string)
	if role != "admin" {
		logError(c, http.StatusForbidden, "Only admin can import system data")
		return
	}

	var backup SystemBackup
	if err := c.ShouldBindJSON(&backup); err != nil {
		logError(c, http.StatusBadRequest, "Invalid backup format: " + err.Error())
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to begin transaction: " + err.Error())
		return
	}

	insertRows := func(table string, rows []map[string]interface{}) error {
		if len(rows) == 0 {
			return nil
		}
		columns := make([]string, 0, len(rows[0]))
		for col := range rows[0] {
			columns = append(columns, col)
		}
		placeholders := make([]string, len(columns))
		for i := range placeholders {
			placeholders[i] = "?"
		}

		stmt, err := tx.Prepare("INSERT INTO " + table + " (" +
			joinStrings(columns, ", ") + ") VALUES (" +
			joinStrings(placeholders, ", ") + ")")
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, row := range rows {
			vals := make([]interface{}, len(columns))
			for i, col := range columns {
				v := row[col]
				if v == nil {
					vals[i] = nil
					continue
				}
				// JSON unmarshals numbers as float64, convert back to int if needed
				if f, ok := v.(float64); ok && f == float64(int64(f)) {
					vals[i] = int64(f)
				} else {
					s, _ := json.Marshal(v)
					vals[i] = string(s)
					if len(s) > 2 && s[0] == '"' {
						vals[i] = v
					}
				}
			}
			if _, err := stmt.Exec(vals...); err != nil {
				return err
			}
		}
		return nil
	}

	// 先清空，再插入
	tableOrder := []string{"shared_connections", "export_logs",  "connections", "users"}
	dataMap := map[string][]map[string]interface{}{
		"users":              backup.Users,
		"connections":        backup.Connections,
		"shared_connections": backup.SharedConnections,
		// "query_logs":         backup.QueryLogs,
		"export_logs":        backup.ExportLogs,
	}

	for _, table := range tableOrder {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			tx.Rollback()
			logError(c, http.StatusInternalServerError, "Failed to clear " + table + ": " + err.Error())
			return
		}
	}

	for _, table := range tableOrder {
		if err := insertRows(table, dataMap[table]); err != nil {
			tx.Rollback()
			logError(c, http.StatusInternalServerError, "Failed to import " + table + ": " + err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		logError(c, http.StatusInternalServerError, "Failed to commit import: " + err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Import completed successfully"})
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}


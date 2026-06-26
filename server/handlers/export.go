// Package handlers 提供导出日志相关的 HTTP 处理函数。
package handlers

import (
	"net/http"
	"sql_panel/server/db"
	"time"

	"github.com/gin-gonic/gin"
)

// ExportLogRequest 保存导出日志的请求体。
type ExportLogRequest struct {
	ConnectionName string `json:"connectionName"`
	SQLStatement   string `json:"sqlStatement"`
	RowCount       int    `json:"rowCount"`
	FileType       string `json:"fileType"`
	FileName       string `json:"fileName"`
}

// ExportLog 导出日志记录。
type ExportLog struct {
	ID             int    `json:"id"`
	UserID         int    `json:"userId"`
	Username       string `json:"username"`
	ConnectionName string `json:"connectionName"`
	SQLStatement   string `json:"sqlStatement"`
	RowCount       int    `json:"rowCount"`
	FileType       string `json:"fileType"`
	FileName       string `json:"fileName"`
	CreatedAt      string `json:"createdAt"`
}

// SaveExportLog 保存一条导出日志记录。
func SaveExportLog(c *gin.Context) {
	userId := c.MustGet("userId").(int)

	// 查询当前用户名
	var username string
	db.DB.QueryRow("SELECT username FROM users WHERE id = ?", userId).Scan(&username)

	var req ExportLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	_, err := db.DB.Exec(
		`INSERT INTO export_logs (user_id, username, connection_name, sql_statement, row_count, file_type, file_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userId, username, req.ConnectionName, req.SQLStatement, req.RowCount, req.FileType, req.FileName, time.Now().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to save export log: " + err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Export log saved"})
}

// GetExportLogs 获取当前用户的导出日志（最近 100 条）。
func GetExportLogs(c *gin.Context) {
	userId := c.MustGet("userId").(int)

	rows, err := db.DB.Query(
		`SELECT id, user_id, username, connection_name, sql_statement, row_count, file_type, file_name, created_at
		 FROM export_logs WHERE user_id = ? ORDER BY created_at DESC LIMIT 100`,
		userId,
	)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to query export logs: " + err.Error())
		return
	}
	defer rows.Close()

	var logs []ExportLog
	for rows.Next() {
		var log ExportLog
		if err := rows.Scan(&log.ID, &log.UserID, &log.Username, &log.ConnectionName, &log.SQLStatement, &log.RowCount, &log.FileType, &log.FileName, &log.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, log)
	}

	// 确保返回空数组而非 null
	if logs == nil {
		logs = []ExportLog{}
	}

	c.JSON(http.StatusOK, logs)
}
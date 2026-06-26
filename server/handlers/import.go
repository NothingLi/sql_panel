// Package handlers 提供文件导入相关的 HTTP 处理函数。
package handlers

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sql_panel/server/db"
	"sql_panel/server/models"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// cachedFile 缓存的项目，只存元数据和临时文件路径，不存数据本身。
type cachedFile struct {
	Columns  []string
	FileName string
	TempPath string // 临时文件路径
	FileExt  string // .csv 或 .xlsx
	ExpireAt time.Time
}

// fileCache 临时文件缓存，数据存磁盘避免撑爆内存。
var (
	fileCacheMu sync.RWMutex
	fileCache   = make(map[string]*cachedFile)
	cacheTTL    = 30 * time.Minute
	tempDir     = ""
)

const (
	importPreviewLimit = 100 // 文件解析预览行数上限
	importBatchSize    = 500 // 批量插入每批行数
)

func init() {
	tempDir = filepath.Join(os.TempDir(), "sql_panel_imports")
	os.MkdirAll(tempDir, 0700)
	go cacheCleaner()
}

// cacheCleaner 每分钟清理过期缓存及对应临时文件。
func cacheCleaner() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		fileCacheMu.Lock()
		for id, cf := range fileCache {
			if time.Now().After(cf.ExpireAt) {
				os.Remove(cf.TempPath)
				delete(fileCache, id)
			}
		}
		fileCacheMu.Unlock()
	}
}

// ParseFileResponse 文件解析响应。
type ParseFileResponse struct {
	FileId    string      `json:"fileId"`
	Columns   []string    `json:"columns"`
	Preview   []StringRow `json:"preview"`
	TotalRows int         `json:"totalRows"`
	FileName  string      `json:"fileName"`
}

// StringRow 字符串类型的行数据。
type StringRow []string

// ImportRequest 导入请求体。
type ImportRequest struct {
	ConnectionId string `json:"connectionId"`
	TableName    string `json:"tableName"`
	FileId       string `json:"fileId"`
	CreateTable  bool   `json:"createTable"`
}

// ParseFile 解析上传的 CSV/XLSX 文件，存临时文件到磁盘，返回预览。
func ParseFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		logError(c, http.StatusBadRequest, "Failed to read uploaded file: " + err.Error())
		return
	}
	defer file.Close()

	fileName := header.Filename
	var fileExt string
	if strings.HasSuffix(strings.ToLower(fileName), ".csv") {
		fileExt = ".csv"
	} else if strings.HasSuffix(strings.ToLower(fileName), ".xlsx") {
		fileExt = ".xlsx"
	} else {
		logError(c, http.StatusBadRequest, "Unsupported file type. Please upload CSV or XLSX file.")
		return
	}

	// 读取全部内容（用于同时写临时文件和解析预览）
	data, err := io.ReadAll(file)
	if err != nil {
		logError(c, http.StatusBadRequest, "Failed to read file: " + err.Error())
		return
	}

	// 解析获取列名和行数
	var columns []string
	var rows []StringRow

	if fileExt == ".csv" {
		columns, rows, err = parseCSVBytes(data)
	} else {
		columns, rows, err = parseXLSXBytes(data)
	}

	if err != nil {
		logError(c, http.StatusBadRequest, "Failed to parse file: " + err.Error())
		return
	}

	if len(columns) == 0 {
		logError(c, http.StatusBadRequest, "File contains no columns")
		return
	}

	// 清理列名中的 BOM 和首尾空白
	for i := range columns {
		columns[i] = strings.TrimSpace(strings.TrimLeft(columns[i], "\uFEFF"))
	}

	// 写入临时文件
	fileId := uuid.New().String()
	tempPath := filepath.Join(tempDir, fileId+fileExt)
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		logError(c, http.StatusInternalServerError, "Failed to store file")
		return
	}

	// 存入缓存（只存元数据）
	fileCacheMu.Lock()
	fileCache[fileId] = &cachedFile{
		Columns:  columns,
		FileName: fileName,
		TempPath: tempPath,
		FileExt:  fileExt,
		ExpireAt: time.Now().Add(cacheTTL),
	}
	fileCacheMu.Unlock()

	// 返回前100行预览
	preview := rows
	if len(preview) > importPreviewLimit {
		preview = preview[:importPreviewLimit]
	}

	c.JSON(http.StatusOK, ParseFileResponse{
		FileId:    fileId,
		Columns:   columns,
		Preview:   preview,
		TotalRows: len(rows),
		FileName:  fileName,
	})
}

// ImportData 从临时文件读取数据并导入到数据库表中。
func ImportData(c *gin.Context) {
	log.Printf("POST import/data started")
	userIdVal, exists := c.Get("userId")
	if !exists {
		logError(c, http.StatusUnauthorized, "userId not found in context")
		return
	}
	userId := userIdVal.(int)

	var req ImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("import/data request: fileId=%s, tableName=%s, connectionId=%s", req.FileId, req.TableName, req.ConnectionId)

	// 从缓存取元数据
	fileCacheMu.RLock()
	cf, ok := fileCache[req.FileId]
	fileCacheMu.RUnlock()

	if !ok {
		logError(c, http.StatusBadRequest, "File data expired or not found, please re-upload")
		return
	}

	// 确保无论导入成功与否，都清理缓存和临时文件
	defer func() {
		fileCacheMu.Lock()
		delete(fileCache, req.FileId)
		fileCacheMu.Unlock()
		os.Remove(cf.TempPath)
		log.Printf("Cleaned up import cache and temp file: %s", req.FileId)
	}()

	// 从临时文件重新解析全量数据
	data, err := os.ReadFile(cf.TempPath)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to read temp file")
		return
	}

	var columns []string
	var rows []StringRow

	if cf.FileExt == ".csv" {
		columns, rows, err = parseCSVBytes(data)
	} else {
		columns, rows, err = parseXLSXBytes(data)
	}

	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to parse file: " + err.Error())
		return
	}

	// 清理列名
	for i := range columns {
		columns[i] = strings.TrimSpace(strings.TrimLeft(columns[i], "\uFEFF"))
	}

	if len(rows) == 0 {
		logError(c, http.StatusBadRequest, "No data to import")
		return
	}

	// 获取连接信息
	connInfo, err := getConnectionByUserID(userId, req.ConnectionId)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to connect to database: " + err.Error())
		return
	}

	safeTableName := quoteIdentifier(req.TableName, connInfo.Type)

	// 如果需要自动建表
	if req.CreateTable {
		createSQL := buildCreateTableSQL(safeTableName, columns, connInfo.Type)
		if _, err := dbConn.Exec(createSQL); err != nil {
			logError(c, http.StatusInternalServerError, "Failed to create table: " + err.Error())
			return
		}
	}

	// 批量插入数据
	start := time.Now()
	totalInserted := 0

	colNames := make([]string, len(columns))
	for i, col := range columns {
		colNames[i] = quoteIdentifier(col, connInfo.Type)
	}

	placeholders := make([]string, len(colNames))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	for i := 0; i < len(rows); i += importBatchSize {
		end := i + importBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]

		valueGroups := make([]string, len(batch))
		for j := range valueGroups {
			valueGroups[j] = "(" + strings.Join(placeholders, ", ") + ")"
		}

		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
			safeTableName,
			strings.Join(colNames, ", "),
			strings.Join(valueGroups, ", "))

		args := make([]interface{}, 0, len(batch)*len(colNames))
		for _, row := range batch {
			for j := 0; j < len(colNames); j++ {
				if j < len(row) {
					args = append(args, row[j])
				} else {
					args = append(args, nil)
				}
			}
		}

		_, err := dbConn.Exec(insertSQL, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to insert rows %d-%d: %s", i+1, end, err.Error()),
			})
			return
		}
		totalInserted += len(batch)
	}

	duration := time.Since(start).String()

	c.JSON(http.StatusOK, gin.H{
		"message":      fmt.Sprintf("Successfully imported %d rows", totalInserted),
		"rowsImported": totalInserted,
		"duration":     duration,
		"tableName":    req.TableName,
	})
}

// parseCSVBytes 从字节切片解析 CSV。
func parseCSVBytes(data []byte) ([]string, []StringRow, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	allRecords, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	if len(allRecords) == 0 {
		return []string{}, []StringRow{}, nil
	}

	columns := allRecords[0]
	var rows []StringRow
	for _, record := range allRecords[1:] {
		rows = append(rows, StringRow(record))
	}

	return columns, rows, nil
}

// parseXLSXBytes 从字节切片解析 XLSX。
func parseXLSXBytes(data []byte) ([]string, []StringRow, error) {
	f, err := excelize.OpenReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		sheetName = "Sheet1"
	}

	allRows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, nil, err
	}

	if len(allRows) == 0 {
		return []string{}, []StringRow{}, nil
	}

	columns := allRows[0]
	var rows []StringRow
	for _, record := range allRows[1:] {
		row := make(StringRow, len(columns))
		copy(row, record)
		rows = append(rows, row)
	}

	return columns, rows, nil
}

// quoteIdentifier 根据数据库类型对标识符加引号。
func quoteIdentifier(name string, dbType models.DBType) string {
	name = strings.TrimSpace(name)
	switch dbType {
	case models.PostgreSQL:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	case models.MySQL:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	case models.SQLite:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	default:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
}

// buildCreateTableSQL 根据数据库类型生成 CREATE TABLE 语句。
func buildCreateTableSQL(tableName string, columns []string, dbType models.DBType) string {
	colDefs := make([]string, len(columns))
	for i, col := range columns {
		colDefs[i] = quoteIdentifier(col, dbType) + " TEXT"
	}

	var engineClause string
	if dbType == models.MySQL {
		engineClause = " ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)%s",
		tableName,
		strings.Join(colDefs, ", "),
		engineClause,
	)
}

// GetAutoColumns 获取目标表中已有的列，用于导入时的列映射。
func GetAutoColumns(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	connectionId := c.Query("connectionId")
	tableName := c.Query("tableName")

	if connectionId == "" || tableName == "" {
		logError(c, http.StatusBadRequest, "connectionId and tableName are required")
		return
	}

	connInfo, err := getConnectionByUserID(userId, connectionId)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	dbConn, err := db.Pool.GetConnection(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to connect: " + err.Error())
		return
	}

	var colQuery string
	switch connInfo.Type {
	case models.SQLite:
		colQuery = fmt.Sprintf("PRAGMA table_info('%s')", strings.ReplaceAll(tableName, "'", "''"))
		rows, err := dbConn.Query(colQuery)
		if err != nil {
			logError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var columns []string
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull int
			var dfltValue, pk interface{}
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
				continue
			}
			columns = append(columns, name)
		}
		c.JSON(http.StatusOK, gin.H{"columns": columns})
		return

	case models.PostgreSQL:
		colQuery = fmt.Sprintf(`
			SELECT column_name
			FROM information_schema.columns
			WHERE table_name = $1
			ORDER BY ordinal_position`)
		rows, err := dbConn.Query(colQuery, tableName)
		if err != nil {
			colQuery = fmt.Sprintf(`
				SELECT column_name
				FROM information_schema.columns
				WHERE table_name = $1 AND table_schema NOT IN ('pg_catalog', 'information_schema')
				ORDER BY ordinal_position`)
			rows, err = dbConn.Query(colQuery, tableName)
		}
		if err != nil {
			logError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var columns []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				continue
			}
			columns = append(columns, name)
		}
		c.JSON(http.StatusOK, gin.H{"columns": columns})
		return

	case models.MySQL:
		colQuery = fmt.Sprintf(`
			SELECT column_name
			FROM information_schema.columns
			WHERE table_name = '%s' AND table_schema = DATABASE()
			ORDER BY ordinal_position`, strings.ReplaceAll(tableName, "'", "''"))
		rows, err := dbConn.Query(colQuery)
		if err != nil {
			logError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var columns []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				continue
			}
			columns = append(columns, name)
		}
		c.JSON(http.StatusOK, gin.H{"columns": columns})
		return

	default:
		logError(c, http.StatusNotImplemented, "Not implemented for " + string(connInfo.Type))
	}
}
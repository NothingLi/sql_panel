// Package handlers 提供数据库连接管理相关的 HTTP 处理函数。
package handlers

import (
	"log"
	"net/http"
	"sql_panel/server/db"
	"sql_panel/server/models"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetUsers 获取用户列表。
// admin 角色返回所有用户，普通用户返回除自己外的其他用户。
func GetUsers(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	role := c.MustGet("role").(string)

	if role == "admin" {
		rows, err := db.DB.Query("SELECT id, username FROM users ORDER BY username")
		if err != nil {
			logError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		users := make([]models.User, 0)
		for rows.Next() {
			var user models.User
			if scanErr := rows.Scan(&user.ID, &user.Username); scanErr == nil {
				users = append(users, user)
			}
		}
		c.JSON(http.StatusOK, users)
		return
	}

	// 普通用户：排除自己
	rows, err := db.DB.Query("SELECT id, username FROM users WHERE id != ? ORDER BY username", userId)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	users := make([]models.User, 0)
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username); err == nil {
			users = append(users, user)
		}
	}

	c.JSON(http.StatusOK, users)
}

// GetConnections 获取当前用户有权限访问的连接列表（通过 shared_connections 表关联）。
func GetConnections(c *gin.Context) {
	userId := c.MustGet("userId").(int)

	query := `
		SELECT c.id, c.name, c.type, c.host, c.port, c.db_user, c.db_password, c.database_name, c.creator_id
		FROM connections c
		JOIN shared_connections sc ON c.id = sc.connection_id
		WHERE sc.user_id = ?`

	rows, err := db.DB.Query(query, userId)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	list := make([]models.Connection, 0)
	for rows.Next() {
		var conn models.Connection
		err := rows.Scan(&conn.ID, &conn.Name, &conn.Type, &conn.Host, &conn.Port, &conn.User, &conn.Password, &conn.DatabaseName, &conn.CreatorID)
		if err == nil {
			conn.Password = "" // 脱敏：不返回明文密码
			list = append(list, conn)
		}
	}
	c.JSON(http.StatusOK, list)
}

// CreateConnection 创建新的数据库连接。
// 同时自动将创建者加入 shared_connections 表中。
func CreateConnection(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	var conn models.Connection

	if err := c.ShouldBindJSON(&conn); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 生成唯一连接 ID
	conn.ID = uuid.New().String()
	conn.CreatorID = userId

	// 使用事务确保连接和权限同时写入
	tx, err := db.DB.Begin()
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = tx.Exec(`
		INSERT INTO connections (id, creator_id, name, type, host, port, db_user, db_password, database_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conn.ID, conn.CreatorID, conn.Name, conn.Type, conn.Host, conn.Port, conn.User, conn.Password, conn.DatabaseName)

	if err != nil {
		tx.Rollback()
		logError(c, http.StatusInternalServerError, "Failed to save connection: " + err.Error())
		return
	}

	// 自动将创建者加入共享表
	_, err = tx.Exec(`INSERT INTO shared_connections (user_id, connection_id, shared_by) VALUES (?, ?, ?)`, userId, conn.ID, userId)
	if err != nil {
		tx.Rollback()
		logError(c, http.StatusInternalServerError, "Failed to assign permission")
		return
	}

	tx.Commit()
	c.JSON(http.StatusCreated, conn)
}

// UpdateConnection 更新数据库连接配置。
// 仅允许连接创建者修改，更新后清除连接池中的旧连接。
func UpdateConnection(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")
	var conn models.Connection
	if err := c.ShouldBindJSON(&conn); err != nil {
		logError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 如果未传入新密码，保留原密码
	if conn.Password == "" {
		var oldPassword string
		err := db.DB.QueryRow("SELECT db_password FROM connections WHERE id=? AND creator_id=?", id, userId).
			Scan(&oldPassword)
		if err != nil {
			logError(c, http.StatusNotFound, "Connection not found or permission denied")
			return
		}
		conn.Password = oldPassword
	}

	result, err := db.DB.Exec(`
		UPDATE connections 
		SET name=?, type=?, host=?, port=?, db_user=?, db_password=?, database_name=?
		WHERE id=? AND creator_id=?`,
		conn.Name, conn.Type, conn.Host, conn.Port, conn.User, conn.Password, conn.DatabaseName, id, userId)

	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("WARNING: RowsAffected failed: %v", err)
	}
	if rowsAffected == 0 {
		logError(c, http.StatusForbidden, "Only creators can modify connection details")
		return
	}

	conn.ID = id
	conn.CreatorID = userId

	// 清除旧连接，下次查询时自动重建
	db.Pool.RemoveConnection(id)

	c.JSON(http.StatusOK, conn)
}

// DeleteConnection 删除数据库连接（仅创建者可删除）。
func DeleteConnection(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	result, err := db.DB.Exec("DELETE FROM connections WHERE id=? AND creator_id=?", id, userId)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("WARNING: RowsAffected failed: %v", err)
	}
	if rowsAffected == 0 {
		logError(c, http.StatusForbidden, "Only creators can delete connections")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})

	// 从连接池中移除
	db.Pool.RemoveConnection(id)
}

// ShareConnection 将连接分享给其他用户。
// 支持单个用户（username）或批量用户（usernames），仅创建者或管理员可分享。
func ShareConnection(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	role := c.MustGet("role").(string)
	id := c.Param("id")

	var input struct {
		TargetUsername  string   `json:"username"`
		TargetUsernames []string `json:"usernames"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		logError(c, http.StatusBadRequest, "Invalid share payload")
		return
	}

	// 兼容单用户和批量两种格式
	targetUsernames := input.TargetUsernames
	if len(targetUsernames) == 0 && input.TargetUsername != "" {
		targetUsernames = []string{input.TargetUsername}
	}
	if len(targetUsernames) == 0 {
		logError(c, http.StatusBadRequest, "At least one target user required")
		return
	}

	// 验证连接存在并获取创建者
	var creatorId int
	err := db.DB.QueryRow("SELECT creator_id FROM connections WHERE id = ?", id).Scan(&creatorId)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found")
		return
	}

	// 权限检查：仅创建者或管理员可分享
	if role != "admin" && creatorId != userId {
		logError(c, http.StatusForbidden, "Only the creator or admin can share this connection")
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to start share transaction")
		return
	}

	sharedUsers := make([]string, 0, len(targetUsernames))
	for _, username := range targetUsernames {
		// 查找目标用户 ID
		var targetId int
		err = tx.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&targetId)
		if err != nil {
			tx.Rollback()
			logError(c, http.StatusNotFound, "Target user not found: " + username)
			return
		}

		_, err = tx.Exec(
			"INSERT OR IGNORE INTO shared_connections (user_id, connection_id, shared_by) VALUES (?, ?, ?)",
			targetId, id, userId,
		)
		if err != nil {
			tx.Rollback()
			logError(c, http.StatusInternalServerError, "Failed to share connection")
			return
		}
		sharedUsers = append(sharedUsers, username)
	}

	if err := tx.Commit(); err != nil {
		logError(c, http.StatusInternalServerError, "Failed to finish sharing")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Shared successfully",
		"users":   sharedUsers,
	})
}

// ShareInfo 连接的分享信息。
type ShareInfo struct {
	UserID           int    `json:"userId"`
	Username         string `json:"username"`
	SharedBy         int    `json:"sharedBy"`
	SharedByUsername string `json:"sharedByUsername"`
	IsCreator        bool   `json:"isCreator"`
}

// GetConnectionShares 获取指定连接的所有分享记录。
func GetConnectionShares(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	role := c.MustGet("role").(string)
	id := c.Param("id")

	var creatorId int
	err := db.DB.QueryRow("SELECT creator_id FROM connections WHERE id = ?", id).Scan(&creatorId)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found")
		return
	}

	// 仅创建者或管理员可查看分享列表
	if role != "admin" && creatorId != userId {
		logError(c, http.StatusForbidden, "Only the creator or admin can view shares")
		return
	}

	rows, err := db.DB.Query(`
		SELECT sc.user_id, u.username, COALESCE(sc.shared_by, 0), COALESCE(su.username, 'system')
		FROM shared_connections sc
		JOIN users u ON sc.user_id = u.id
		LEFT JOIN users su ON sc.shared_by = su.id
		WHERE sc.connection_id = ?
		ORDER BY u.username`, id)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	shares := make([]ShareInfo, 0)
	for rows.Next() {
		var s ShareInfo
		if err := rows.Scan(&s.UserID, &s.Username, &s.SharedBy, &s.SharedByUsername); err == nil {
			s.IsCreator = s.UserID == creatorId
			shares = append(shares, s)
		}
	}

	c.JSON(http.StatusOK, shares)
}

// RevokeShare 撤销对某个用户的连接分享。
// 管理员可撤销任意分享，普通用户只能撤销自己创建的分享。
func RevokeShare(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	role := c.MustGet("role").(string)
	connID := c.Param("id")
	targetUserID := c.Param("userId")

	// 验证连接存在
	var creatorId int
	err := db.DB.QueryRow("SELECT creator_id FROM connections WHERE id = ?", connID).Scan(&creatorId)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found")
		return
	}

	targetUID, err := strconv.Atoi(targetUserID)
	if err != nil {
		logError(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// 不允许撤销创建者的访问权限
	if targetUID == creatorId {
		logError(c, http.StatusBadRequest, "Cannot revoke the creator's access")
		return
	}

	// 管理员可直接撤销任意分享
	if role == "admin" {
		_, err = db.DB.Exec("DELETE FROM shared_connections WHERE connection_id = ? AND user_id = ?", connID, targetUID)
		if err != nil {
			logError(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Access revoked"})
		return
	}

	// 普通用户只能撤销自己创建的分享
	var sharedBy int
	err = db.DB.QueryRow(
		"SELECT COALESCE(shared_by, 0) FROM shared_connections WHERE connection_id = ? AND user_id = ?",
		connID, targetUID,
	).Scan(&sharedBy)
	if err != nil {
		logError(c, http.StatusNotFound, "Share record not found")
		return
	}

	if sharedBy != userId {
		logError(c, http.StatusForbidden, "You can only revoke shares you created")
		return
	}

	_, err = db.DB.Exec("DELETE FROM shared_connections WHERE connection_id = ? AND user_id = ?", connID, targetUID)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Access revoked"})
}

// ConnectionWithShareCount 带分享数量的连接信息（管理员视图）。
type ConnectionWithShareCount struct {
	models.Connection
	ShareCount int `json:"shareCount"`
}

// GetAllConnections 管理员获取所有连接及其分享数量。
func GetAllConnections(c *gin.Context) {
	role := c.MustGet("role").(string)
	if role != "admin" {
		logError(c, http.StatusForbidden, "Admin only")
		return
	}

	rows, err := db.DB.Query(`
		SELECT c.id, c.name, c.type, c.host, c.port, c.db_user, c.db_password, c.database_name, c.creator_id,
			(SELECT COUNT(*) FROM shared_connections sc WHERE sc.connection_id = c.id) as share_count
		FROM connections c
		ORDER BY c.name`)
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	list := make([]ConnectionWithShareCount, 0)
	for rows.Next() {
		var item ConnectionWithShareCount
		err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.Host, &item.Port, &item.User, &item.Password, &item.DatabaseName, &item.CreatorID, &item.ShareCount)
		if err == nil {
			item.Password = "" // 脱敏：不返回明文密码
			list = append(list, item)
		}
	}
	c.JSON(http.StatusOK, list)
}

// GetConnectionVersion 探测指定连接的数据库版本（主版本号）。
// 用于前端函数智能提示按版本过滤。结果在连接池中缓存，连接移除时自动失效。
func GetConnectionVersion(c *gin.Context) {
	userId := c.MustGet("userId").(int)
	id := c.Param("id")

	connInfo, err := getConnectionByUserID(userId, id)
	if err != nil {
		logError(c, http.StatusNotFound, "Connection not found or permission denied")
		return
	}

	version, err := db.Pool.GetVersion(connInfo)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to detect database version: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":    connInfo.Type,
		"version": version,
	})
}
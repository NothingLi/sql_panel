// Package handlers 提供用户认证相关的 HTTP 处理函数，包括注册、登录和用户管理。
package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sql_panel/server/config"
	"sql_panel/server/db"
	"sql_panel/server/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// jwtKey JWT 签名密钥，统一通过 config 包获取。
// 生产环境应通过环境变量 JWT_SECRET 设置。

// Register 处理用户注册请求。
// 接收 username 和 password，使用 bcrypt 加密密码后存入数据库。
func Register(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		logError(c, http.StatusBadRequest, "Invalid input")
		return
	}

	// 使用 bcrypt 对密码进行哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// 写入用户表
	_, err = db.DB.Exec("INSERT INTO users (username, password) VALUES (?, ?)", input.Username, string(hashedPassword))
	if err != nil {
		logError(c, http.StatusBadRequest, "Username already exists")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
}

// CreateUser 管理员创建新用户（角色固定为 user）。
func CreateUser(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" {
		logError(c, http.StatusForbidden, "Only admin can create users")
		return
	}

	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		logError(c, http.StatusBadRequest, "Invalid input")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	_, err = db.DB.Exec("INSERT INTO users (username, password, role) VALUES (?, ?, 'user')", input.Username, string(hashedPassword))
	if err != nil {
		logError(c, http.StatusBadRequest, "Username already exists")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User created successfully"})
}

// ListUsers 管理员获取所有用户列表（id, username, role）。
func ListUsers(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" {
		logError(c, http.StatusForbidden, "Only admin can view all users")
		return
	}

	rows, err := db.DB.Query("SELECT id, username, role FROM users ORDER BY id")
	if err != nil {
		logError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	users := make([]models.User, 0)
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Role); err == nil {
			users = append(users, user)
		}
	}

	c.JSON(http.StatusOK, users)
}

// UpdateUser 管理员更新用户信息（用户名、密码、角色）。
// 不允许管理员降级自己的角色。
func UpdateUser(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" {
		logError(c, http.StatusForbidden, "Only admin can update users")
		return
	}

	userId := c.MustGet("userId").(int)
	targetId := c.Param("id")

	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		logError(c, http.StatusBadRequest, "Invalid input")
		return
	}

	targetIdInt := 0
	if _, err := fmt.Sscanf(targetId, "%d", &targetIdInt); err != nil || targetIdInt == 0 {
		logError(c, http.StatusBadRequest, "Invalid user id")
		return
	}

	// 禁止管理员将自己降级为非 admin
	if targetIdInt == userId && input.Role != "" && input.Role != "admin" {
		logError(c, http.StatusBadRequest, "Cannot demote yourself")
		return
	}

	// 更新用户名
	if input.Username != "" {
		_, err := db.DB.Exec("UPDATE users SET username = ? WHERE id = ?", input.Username, targetIdInt)
		if err != nil {
			logError(c, http.StatusBadRequest, "Username already exists or update failed")
			return
		}
	}

	// 更新密码（如果提供了新密码）
	if input.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			logError(c, http.StatusInternalServerError, "Failed to hash password")
			return
		}
		db.DB.Exec("UPDATE users SET password = ? WHERE id = ?", string(hashedPassword), targetIdInt)
	}

	// 更新角色
	if input.Role != "" {
		db.DB.Exec("UPDATE users SET role = ? WHERE id = ?", input.Role, targetIdInt)
	}

	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})
}

// DeleteUser 管理员删除用户（不允许删除自己）。
func DeleteUser(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" {
		logError(c, http.StatusForbidden, "Only admin can delete users")
		return
	}

	userId := c.MustGet("userId").(int)
	targetId := c.Param("id")

	targetIdInt := 0
	if _, err := fmt.Sscanf(targetId, "%d", &targetIdInt); err != nil || targetIdInt == 0 {
		logError(c, http.StatusBadRequest, "Invalid user id")
		return
	}

	// 禁止管理员删除自己
	if targetIdInt == userId {
		logError(c, http.StatusBadRequest, "Cannot delete yourself")
		return
	}

	result, err := db.DB.Exec("DELETE FROM users WHERE id = ?", targetIdInt)
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("WARNING: RowsAffected failed: %v", err)
	}
	if rowsAffected == 0 {
		logError(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

// Login 处理用户登录请求。
// 验证用户名密码后生成 JWT Token，有效期为 24 小时。
func Login(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		logError(c, http.StatusBadRequest, "Invalid input")
		return
	}

	// 1. 查找用户
	var user models.User
	err := db.DB.QueryRow("SELECT id, username, password, role FROM users WHERE username = ?", input.Username).
		Scan(&user.ID, &user.Username, &user.Password, &user.Role)

	if err == sql.ErrNoRows {
		logError(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// 2. 验证密码（bcrypt 比对）
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		logError(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// 3. 生成 JWT Token，payload 包含 userId 和 role
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": user.ID,
		"role":   user.Role,
		"exp":    time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString(config.JWTSecret())
	if err != nil {
		logError(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":    tokenString,
		"username": user.Username,
		"userId":   user.ID,
		"role":     user.Role,
	})
}
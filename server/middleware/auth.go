// Package middleware 提供 Gin 中间件，包括 JWT 认证和 AES 报文加解密。
package middleware

import (
	"fmt"
	"net/http"
	"sql_panel/server/config"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWT 签名/验证密钥，统一通过 config 包获取。

// AuthMiddleware JWT 认证中间件。
// 从 Authorization 头中提取 Bearer Token，验证签名和有效期，
// 验证通过后将 userId 和 role 注入请求上下文，供后续 handler 使用。
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 获取 Authorization Header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// 2. 解析 Bearer Token 格式
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 3. 验证 Token 签名和有效性
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// 确保签名算法为 HMAC
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return config.JWTSecret(), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// 4. 提取 Claims 中的用户信息，注入上下文
		claims, ok := token.Claims.(jwt.MapClaims)
		if ok && token.Valid {
			userId := int(claims["userId"].(float64))
			c.Set("userId", userId)
			if role, exists := claims["role"]; exists {
				c.Set("role", role.(string))
			}
		}
		c.Next()
	}
}
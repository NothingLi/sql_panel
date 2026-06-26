package handlers

import (
	"log"

	"github.com/gin-gonic/gin"
)

// logError 统一记录错误日志并返回 JSON 错误响应。
func logError(c *gin.Context, code int, errMsg string) {
	log.Printf("%s %s: %s", c.Request.Method, c.Request.URL.Path, errMsg)
	c.JSON(code, gin.H{"error": errMsg})
}
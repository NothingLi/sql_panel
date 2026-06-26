// Package middleware 提供 Gin 中间件，包括 JWT 认证和 AES 报文加解密。
package middleware

import (
	"bytes"
	"io"
	"net/http"
	"sql_panel/server/crypto"
	"strings"

	"github.com/gin-gonic/gin"
)

// encryptResponseWriter 包装 Gin 的 ResponseWriter，拦截响应体用于加密。
// 将业务 handler 写入的数据暂存到 body 缓冲区，在中间件返回前统一加密输出。
type encryptResponseWriter struct {
	gin.ResponseWriter
	body       *bytes.Buffer // 暂存响应体明文
	statusCode int           // 暂存 HTTP 状态码
}

// Write 拦截写入操作，将数据写入内部缓冲区而非直接发送。
func (w *encryptResponseWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

// WriteString 拦截字符串写入操作。
func (w *encryptResponseWriter) WriteString(s string) (int, error) {
	return w.body.WriteString(s)
}

// WriteHeader 暂存状态码，延迟到加密后再真正写入。
func (w *encryptResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// skipEncryptionPaths 定义不需要加密的接口路径
var skipEncryptionPaths = []string{}

// shouldSkipEncryption 判断是否应该跳过加密
func shouldSkipEncryption(path string) bool {
	for _, p := range skipEncryptionPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// EncryptMiddleware 创建报文加解密中间件。
// 请求阶段：解密请求体（Base64 密文 → 明文 JSON）。
// 响应阶段：加密 JSON 响应体（明文 JSON → Base64 密文）。
// 非 JSON 响应（如文件下载）不加密，原样返回。
// 导入导出接口始终不加密，以便用户直接查看和编辑导出文件。
func EncryptMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 导入导出接口跳过加密处理
		if shouldSkipEncryption(c.Request.URL.Path) {
			c.Next()
			return
		}

		// ========== 请求解密 ==========
		// GET 请求没有 body，跳过解密
		if c.Request.Body != nil && c.Request.Method != http.MethodGet {
			body, err := io.ReadAll(c.Request.Body)
			c.Request.Body.Close()
			if err == nil && len(body) > 0 {
				// 尝试解密，成功则替换 body 为明文并修正 Content-Type
				if plaintext, err := crypto.Decrypt(string(body)); err == nil {
					c.Request.Body = io.NopCloser(bytes.NewBuffer(plaintext))
					c.Request.Header.Set("Content-Type", "application/json")
				} else {
					// 解密失败则保留原始 body（兼容非加密请求）
					c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
				}
			} else {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
			}
		}

		// ========== 响应加密 ==========
		// 用自定义 Writer 替换默认 Writer，拦截响应输出
		writer := &encryptResponseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}
		c.Writer = writer

		// 执行后续 handler
		c.Next()

		// 只对 JSON 响应加密，文件下载等非 JSON 响应原样返回
		contentType := writer.Header().Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			if writer.body.Len() > 0 {
				writer.ResponseWriter.WriteHeader(writer.statusCode)
				writer.ResponseWriter.Write(writer.body.Bytes())
			} else {
				writer.ResponseWriter.WriteHeader(writer.statusCode)
			}
			return
		}

		// 加密 JSON 响应体，修改 Content-Type 为 text/plain
		if writer.body.Len() > 0 {
			encrypted, err := crypto.Encrypt(writer.body.Bytes())
			if err == nil {
				writer.Header().Set("Content-Type", "text/plain")
				writer.ResponseWriter.WriteHeader(writer.statusCode)
				writer.ResponseWriter.Write([]byte(encrypted))
			} else {
				// 加密失败则返回明文（降级处理）
				writer.ResponseWriter.WriteHeader(writer.statusCode)
				writer.ResponseWriter.Write(writer.body.Bytes())
			}
		} else {
			writer.ResponseWriter.WriteHeader(writer.statusCode)
		}
	}
}
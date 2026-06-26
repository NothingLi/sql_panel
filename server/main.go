// Package main 是 SQL Panel 的入口文件，负责启动 Web 服务器。
// 提供数据库连接管理、SQL 查询执行、数据导出、用户管理等功能的 Web 面板。
package main

import (
		"net/http"
	"fmt"
	"log"
	"os"
	"sql_panel/server/db"
	"sql_panel/server/handlers"
	"sql_panel/server/middleware"
	"embed"
	"io/fs"
	"strings"
	"github.com/gin-gonic/gin"
)

//go:embed dist/*
var staticFiles embed.FS

func injectEncryptScript(html []byte) []byte {
	encryptEnabled := os.Getenv("ENCRYPT")
	script := fmt.Sprintf(`<script>window.__ENCRYPT__="%s";</script></head>`, encryptEnabled)
	return []byte(strings.Replace(string(html), "</head>", script, 1))
}

func main() {
	// 初始化系统数据库（默认 SQLite，可通过环境变量切换为 MySQL/PostgreSQL）
	db.InitDB()

	// 创建 Gin 引擎
	r := gin.Default()


	distFS, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		panic("无法加载前端文件: " + err.Error())
	}

	// ============ 前端中间件（必须放在路由定义之前）============
	r.Use(func(c *gin.Context) {
		path := c.Request.URL.Path

		// 只处理 /sqlpanel 开头的非 API 请求
		if !strings.HasPrefix(path, "/sqlpanel") {
			c.Next()
			return
		}

		relativePath := strings.TrimPrefix(path, "/sqlpanel")
		relativePath = strings.TrimPrefix(relativePath, "/")

		// API 请求跳过，让路由组处理
		if strings.HasPrefix(relativePath, "api") {
			c.Next()
			return
		}

		// 根路径，返回 index.html
		if relativePath == "" {
			data, _ := staticFiles.ReadFile("dist/index.html")
			c.Data(http.StatusOK, "text/html; charset=utf-8", injectEncryptScript(data))
			c.Abort()
			return
		}

		// 尝试提供静态文件
		f, err := distFS.Open(relativePath)
		if err == nil {
			f.Close()
			c.Request.URL.Path = "/" + relativePath
			http.FileServer(http.FS(distFS)).ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		// SPA 路由：返回 index.html
		data, _ := staticFiles.ReadFile("dist/index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", injectEncryptScript(data))
		c.Abort()
	})



	panel := r.Group("/sqlpanel")
	{
		// 受保护的 API 路由组，需要 JWT 认证
		api := panel.Group("/api")
		encrypt := os.Getenv("ENCRYPT")
		if encrypt == "true" {
			api.Use(middleware.EncryptMiddleware())
		}
		{
			// 	// 注册用户注册和登录路由（无需认证）
			// api.POST("/api/register", handlers.Register)
			api.POST("/login", handlers.Login)
			api.GET("/ping", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "pong"})
			})
			auth := api.Group("")
			auth.Use(middleware.AuthMiddleware())
			{
				// 用户管理（admin 权限）
				auth.POST("/users", handlers.CreateUser)
				auth.GET("/users/list", handlers.ListUsers)
				auth.PUT("/users/:id", handlers.UpdateUser)
				auth.DELETE("/users/:id", handlers.DeleteUser)

				// 获取用户列表（用于分享弹窗）
				auth.GET("/users", handlers.GetUsers)

				// 数据库连接 CRUD
				auth.GET("/connections", handlers.GetConnections)
				auth.POST("/connections", handlers.CreateConnection)
				auth.PUT("/connections/:id", handlers.UpdateConnection)
				auth.DELETE("/connections/:id", handlers.DeleteConnection)

				// 连接分享与撤销
				auth.POST("/connections/:id/share", handlers.ShareConnection)
				auth.GET("/connections/:id/shares", handlers.GetConnectionShares)
				auth.DELETE("/connections/:id/shares/:userId", handlers.RevokeShare)

				// 管理员获取所有连接
				auth.GET("/connections/all", handlers.GetAllConnections)

				// 统一查询入口（自动分发 SELECT/INSERT/事务）
				auth.POST("/query", handlers.ExecuteQuery)

				// 数据库元数据查询
				auth.GET("/tables/:id", handlers.GetTables)
				auth.GET("/tables/:id/:table/columns", handlers.GetTableColumns)
				auth.GET("/tables/:id/:table/ddl", handlers.GetTableDDL)
				auth.GET("/connections/:id/schema/:table/ddl", handlers.GetTableDDL)

				// SQL 格式化
				auth.POST("/format-sql", handlers.FormatSQLHandler)

				// // 查询日志
				// auth.POST("/query-logs", handlers.SaveQueryLog)
				// auth.GET("/query-logs", handlers.GetQueryLogs)

				// 导出日志
				auth.POST("/export-logs", handlers.SaveExportLog)
				auth.GET("/export-logs", handlers.GetExportLogs)

				// 系统数据导入导出
				auth.GET("/system/export", handlers.ExportSystemData)
				auth.POST("/system/import", handlers.ImportSystemData)

				// 文件数据导入
				auth.POST("/import/parse", handlers.ParseFile)
				auth.POST("/import/data", handlers.ImportData)
				auth.GET("/import/columns", handlers.GetAutoColumns)
			}
		}
	}
	

	// 从环境变量读取端口，默认 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// 根路径重定向
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/sqlpanel")
	})

	// 404 处理
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "路径不存在"})
	})
	// 打印启动信息
	fmt.Printf("🚀 SQL Panel is running on http://localhost:%s\n", port)
	fmt.Printf("📝 Web UI: http://localhost:%s\n", port)
	fmt.Printf("🔧 API  : http://localhost:%s/api\n", port)

	// 启动 HTTP 服务器
	log.Fatal(r.Run(":" + port))
}
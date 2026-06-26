package handlers

import (
	"sql_panel/server/db"
	"sql_panel/server/models"
)

// getConnectionByUserID 根据用户 ID 和连接 ID 查询数据库连接。
// 通过 shared_connections 验证用户是否有访问权限。
func getConnectionByUserID(userId int, connID string) (models.Connection, error) {
	var conn models.Connection
	query := `
		SELECT c.id, c.name, c.type, c.host, c.port, c.db_user, c.db_password, c.database_name
		FROM connections c
		JOIN shared_connections sc ON c.id = sc.connection_id
		WHERE sc.user_id = ? AND c.id = ? LIMIT 1`
	err := db.DB.QueryRow(query, userId, connID).
		Scan(&conn.ID, &conn.Name, &conn.Type, &conn.Host, &conn.Port, &conn.User, &conn.Password, &conn.DatabaseName)
	if err != nil {
		return conn, err
	}
	return conn, nil
}
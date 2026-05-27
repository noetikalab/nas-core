package handler

import (
	"net/http"

	"nas/ldap"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// ListUsers 返回 LDAP 中的所有用户，包含用户名、UID、GID、家目录和角色。
//
// 角色来自 LDAP employeeType 属性，可能的值：
//   - "admin"：管理员，可通过 /api/* 路由访问所有管理功能
//   - "user"：普通用户，只能访问自己的文件
func ListUsers(c *gin.Context) {
	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "ldap unavailable"})
		return
	}
	defer conn.Close()

	// 从 LDAP 查所有 posixAccount 用户
	users, err := ldap.SearchUsers(conn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "user search failed"})
		return
	}

	// 将 LDAP 层的 UserInfo 转换为 handler 层的响应 DTO
	entries := make([]UserEntry, len(users))
	for i, u := range users {
		entries[i] = UserEntry{
			Username: u.Username,
			UID:      u.UID,
			GID:      u.GID,
			Home:     u.Home,
			Role:     u.Role,
		}
	}

	c.JSON(http.StatusOK, UserListResponse{Users: entries})
}

// DeleteUser 删除用户：先从 LDAP 移除条目，再清理 Linux 系统和数据目录。
//
// 删除顺序极为重要：
//   - 必须先查 UID（在 LDAP 条目还存在时），因为 userdel 需要 UID
//   - 再删 LDAP 条目（如果失败则中止，不执行 Linux 清理）
//   - 最后 best-effort 清理 Linux 用户和数据目录
//
// URL 参数：username（路径参数，如 /api/users/alice）
func DeleteUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "username required"})
		return
	}

	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "ldap unavailable"})
		return
	}
	defer conn.Close()

	// 第一步：删除前先读取 UID（LDAP 条目还在）
	uid, _ := ldap.GetUID(conn, username)

	// 第二步：从 LDAP 删除用户条目
	if err := ldap.DeleteUser(conn, username); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "ldap delete failed"})
		return
	}

	// 第三步：best-effort 清理 Linux 系统
	// 即使失败了也不要紧，此时 LDAP 条目已删除，用户无法再登录
	system.DeleteUser(username, uid)

	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// CountUsers 返回 LDAP 中的用户总数。
// 主要供 Register handler 在注册时调用，判断是否为第一个用户（自动授予 admin 角色）。
func CountUsers(c *gin.Context) {
	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "ldap unavailable"})
		return
	}
	defer conn.Close()

	count, err := ldap.CountUsers(conn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "count failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

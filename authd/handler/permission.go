package handler

import (
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"nas/ldap"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// SetPermission
// @Summary      Set file share permission
// @Description  Grant or revoke POSIX ACL access for another user.
// @Description  Actions: "readonly" (r-x), "readwrite" (rwx), "remove" (revoke all).
// @Tags         share
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body SetPermissionRequest true "Permission details"
// @Success      200 {object} OKResponse "Permission set"
// @Failure      400 {object} ErrorResponse "Invalid input"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Router       /api/share/permission [post]
func SetPermission(c *gin.Context) {
	var req SetPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	if req.Action == "remove" {
		system.RemoveACL(req.Path, req.TargetUser)
		c.JSON(http.StatusOK, OKResponse{OK: true})
		return
	}

	perm := "rwx"
	if req.Action == "readonly" || req.Readonly {
		perm = "r-x"
	}
	system.SetACL(req.Path, req.TargetUser, perm)
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// QueryPermissions 查询指定路径上的 POSIX ACL 权限列表。
// 解析 getfacl 输出，提取 user:xxx:perm 条目，忽略默认 ACL、mask、group 等系统条目。
//
// @Summary      Query ACL permissions for a path
// @Description  Return the list of user-specific ACL entries on the given path.
// @Tags         share
// @Produce      json
// @Security     BearerAuth
// @Param        path query string true "Path to query"
// @Success      200 {object} PermissionListResponse "ACL entries"
// @Failure      400 {object} ErrorResponse "Path required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      500 {object} ErrorResponse "ACL query failed"
// @Router       /api/share/permissions [get]
func QueryPermissions(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	// 调用系统 getfacl 命令，仅在绝对必要（读取 Linux 文件系统 ACL）时使用
	out, err := exec.Command("getfacl", "-c", path).CombinedOutput()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("getfacl failed: %v", err)})
		return
	}

	// 解析 getfacl 输出，格式如:
	//   user:alice:rwx
	//   user:bob:r-x
	//   group::r-x
	//   mask::rwx
	// 只提取 "user:" 开头的行（非 owner），忽略 "group:"、"mask:"、"other:" 等
	entries := []PermissionEntry{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "user:") {
			continue
		}
		// 跳过 owner 条目（格式为 "user::rwx"，双冒号表示文件 owner）
		if strings.HasPrefix(line, "user::") {
			continue
		}
		// 格式: "user:username:perm"
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 {
			username := parts[1]
			// getfacl 可能返回 UID 数字而非用户名（LDAP 用户不在 /etc/passwd 时）
			// 尝试通过 LDAP 反查 UID 对应的真实用户名
			if uid, err := strconv.Atoi(username); err == nil && uid > 0 {
				if name := resolveUsernameByUID(uid); name != "" {
					username = name
				}
			}
			entries = append(entries, PermissionEntry{
				Username:   username,
				Permission: parts[2],
			})
		}
	}

	c.JSON(http.StatusOK, PermissionListResponse{
		Path:        path,
		Permissions: entries,
	})
}

// resolveUsernameByUID 通过 LDAP 查询 UID 对应的用户名。
// getfacl 对 LDAP 用户可能返回 UID 数字而非用户名（/etc/passwd 中无此用户时），
// 此函数通过现有 SearchUsers API 反查用户名。
// 查询失败或用户不存在时返回空字符串，调用方保持原始 UID 值。
func resolveUsernameByUID(uid int) string {
	conn, err := ldap.Conn()
	if err != nil {
		return ""
	}
	defer conn.Close()

	users, err := ldap.SearchUsers(conn)
	if err != nil {
		return ""
	}
	for _, u := range users {
		if u.UID == uid {
			return u.Username
		}
	}
	return ""
}

package system

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"

	"nas/ldap"
)

// CreateUser 在 Linux 系统上创建用户。
//
// useradd 参数说明：
//   - -u {uid}：指定 UID，与 LDAP uidNumber 一致
//   - -g 1000：主组固定为 nasgroup（GID=1000）
//   - -M：不创建家目录（目录由 CreateDataDir 单独创建和控制权限）
//   - -s /sbin/nologin：禁止 shell 登录（仅 SMB/NFS/WebDAV 访问）
//
// 注意：useradd 失败时不回滚 LDAP（调用方需处理两阶段一致性）。
func CreateUser(username string, uid int) error {
	out, err := exec.Command("useradd",
		"-u", strconv.Itoa(uid),
		"-g", "1000",
		"-M",
		"-s", "/sbin/nologin",
		username,
	).CombinedOutput()
	if err != nil {
		log.Printf("useradd failed for %s: %v: %s", username, err, out)
	}
	return err
}

// CreateDataDir 创建用户数据目录 /data/{username}，设置正确的 owner 和权限。
// 目录权限 700：只有 owner 能访问，其他用户需通过 POSIX ACL 授权。
func CreateDataDir(username string, uid int) {
	exec.Command("mkdir", "-p", "/data/"+username).Run()
	exec.Command("chown", fmt.Sprintf("%d:1000", uid), "/data/"+username).Run()
	exec.Command("chmod", "700", "/data/"+username).Run()
}

// resolveACLUser 将用户名转换为 setfacl 可接受的标识符。
// setfacl 通过 NSS 解析用户名，但 LDAP 用户对容器内的 setfacl 不可见，
// 因此必须将用户名转换为数字 UID（查询 LDAP）。
// 查询失败时返回原始用户名作为 fallback。
func resolveACLUser(username string) string {
	conn, err := ldap.Conn()
	if err != nil {
		log.Printf("resolveACLUser: ldap connect failed for %s: %v", username, err)
		return username // fallback
	}
	defer conn.Close()

	uid, _ := ldap.GetUID(conn, username)
	if uid > 0 {
		return strconv.Itoa(uid)
	}
	log.Printf("resolveACLUser: uid not found for %s, using raw username", username)
	return username
}

// SetACL 递归设置 POSIX ACL，为目标用户授予指定目录的访问权限。
//
// 两个关键步骤缺一不可：
//   - chmod g+x {path}：确保 Samba 在检查 POSIX ACL 之前不会拒绝目录访问
//   - setfacl -R -m user:{target}:{perm}：设置 ACL 条目
//   - setfacl -R -d -m …：设置默认 ACL（新建子文件/目录自动继承）
//
// perm 取值：rwx（读写执行）、r-x（只读）、---（无权限）
// targetUser 经 resolveACLUser 转换为数字 UID，解决容器内 setfacl 无法解析 LDAP 用户名的问题。
func SetACL(path, targetUser, perm string) {
	userID := resolveACLUser(targetUser)
	entry := fmt.Sprintf("user:%s:%s", userID, perm)

	// Samba 权限预检：目录必须有 group execute 权限，否则在 ACL 检查前就返回拒绝
	if out, err := exec.Command("chmod", "g+x", path).CombinedOutput(); err != nil {
		log.Printf("SetACL chmod failed for %s (%s): %v: %s", path, targetUser, err, out)
	}
	if out, err := exec.Command("setfacl", "-R", "-m", entry, path).CombinedOutput(); err != nil {
		log.Printf("SetACL setfacl failed for %s (%s): %v: %s", path, targetUser, err, out)
	}
	if out, err := exec.Command("setfacl", "-R", "-d", "-m", entry, path).CombinedOutput(); err != nil {
		log.Printf("SetACL setfacl default failed for %s (%s): %v: %s", path, targetUser, err, out)
	}
}

// RemoveACL 递归移除目标用户在某路径上的所有 ACL 条目（含默认 ACL）。
// targetUser 经 resolveACLUser 转换为数字 UID，原因同 SetACL。
func RemoveACL(path, targetUser string) {
	userID := resolveACLUser(targetUser)

	if out, err := exec.Command("setfacl", "-R", "-x", fmt.Sprintf("user:%s", userID), path).CombinedOutput(); err != nil {
		log.Printf("RemoveACL setfacl failed for %s (%s): %v: %s", path, targetUser, err, out)
	}
	if out, err := exec.Command("setfacl", "-R", "-x", fmt.Sprintf("default:user:%s", userID), path).CombinedOutput(); err != nil {
		log.Printf("RemoveACL setfacl default failed for %s (%s): %v: %s", path, targetUser, err, out)
	}
}

// DeleteUser 删除 Linux 系统用户及其数据目录。
// 这是 best-effort 操作：即使命令失败也不返回错误，
// 因为 LDAP 条目可能已被成功删除，这里的失败只是"数据未清理干净"。
//
// userdel -r 会删除用户和其家目录，但由于我们使用 -M 创建用户（无家目录），
// 额外用 rm -rf 确保 /data/{username} 被清理。
func DeleteUser(username string, uid int) {
	exec.Command("userdel", "-r", username).Run()
	exec.Command("rm", "-rf", "/data/"+username).Run()
}

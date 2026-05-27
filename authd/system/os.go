package system

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
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

// SetACL 递归设置 POSIX ACL，为目标用户授予指定目录的访问权限。
//
// 两个关键步骤缺一不可：
//   - chmod g+x {path}：确保 Samba 在检查 POSIX ACL 之前不会拒绝目录访问
//   - setfacl -R -m user:{target}:{perm}：设置 ACL 条目
//   - setfacl -R -d -m …：设置默认 ACL（新建子文件/目录自动继承）
//
// perm 取值：rwx / rwx（读写执行）、r-x（只读）、---
func SetACL(path, targetUser, perm string) {
	entry := fmt.Sprintf("user:%s:%s", targetUser, perm)
	// Samba 权限预检：目录必须有 group execute 权限，否则在 ACL 检查前就返回拒绝
	exec.Command("chmod", "g+x", path).Run()
	exec.Command("setfacl", "-R", "-m", entry, path).Run()
	exec.Command("setfacl", "-R", "-d", "-m", entry, path).Run()
}

// RemoveACL 递归移除目标用户在某路径上的所有 ACL 条目（含默认 ACL）。
func RemoveACL(path, targetUser string) {
	exec.Command("setfacl", "-R", "-x", fmt.Sprintf("user:%s", targetUser), path).Run()
	exec.Command("setfacl", "-R", "-x", fmt.Sprintf("default:user:%s", targetUser), path).Run()
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

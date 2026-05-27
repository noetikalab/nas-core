// Package ldap 封装 OpenLDAP 操作，是 NAS 系统的唯一身份数据源。
//
// 职责范围：
//   - 连接管理（DialURL + Admin Bind）
//   - 用户增删改查（AddUser、DeleteUser、SearchUsers、GetUserRole、CountUsers）
//   - 认证（Bind 密码验证）
//   - Samba 属性计算（NT hash、ssha）
//
// 用户角色通过 LDAP employeeType 属性存储（"admin" | "user"），
// 与注册流程一致，不依赖环境变量或外部配置。
package ldap

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
	"unicode/utf16"

	"github.com/go-ldap/ldap/v3"

	// MD4 仅用于 Samba NT hash 计算，是 SMB 协议要求，不用于安全用途。
	// Samba ldapsam 后端需要 NT hash 才能完成 NTLM 认证。
	"golang.org/x/crypto/md4"
)

// 全局 LDAP 配置，由 main.go 从环境变量读取后注入。
var (
	URL       string // LDAP 服务地址，如 ldap://openldap:389
	AdminDN   string // 管理员 DN，如 cn=admin,dc=nas,dc=local
	AdminPW   string // 管理员密码
	UsersDN   string // 用户基础 DN，如 ou=users,dc=nas,dc=local
	DomainSID string // Samba 域 SID，用于生成 sambaSID
)

// Conn 创建 LDAP 连接并以管理员身份绑定。
// 调用方负责在使用完毕后 conn.Close()。
func Conn() (*ldap.Conn, error) {
	conn, err := ldap.DialURL(URL)
	if err != nil {
		return nil, err
	}
	if err = conn.Bind(AdminDN, AdminPW); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Bind 以指定用户的身份进行 LDAP 绑定（密码验证）。
// 返回 nil 表示用户名和密码正确。
func Bind(username, password string) error {
	conn, err := ldap.DialURL(URL)
	if err != nil {
		return err
	}
	defer conn.Close()
	// 用户 DN 格式：uid={username},ou=users,dc=nas,dc=local
	return conn.Bind(fmt.Sprintf("uid=%s,%s", username, UsersDN), password)
}

// NextUID 查找当前最大 uidNumber 并返回 max+1。
// 遍历所有 posixAccount 条目，确保 UID 唯一。
// 起始 UID 为 1001（1000 保留给 nasgroup）。
func NextUID(conn *ldap.Conn) (int, error) {
	res, err := conn.Search(ldap.NewSearchRequest(
		UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=posixAccount)", []string{"uidNumber"}, nil,
	))
	if err != nil {
		return 0, err
	}
	max := 1000 // 从 1000 开始，第一个用户 UID=1001
	for _, e := range res.Entries {
		if uid, err := strconv.Atoi(e.GetAttributeValue("uidNumber")); err == nil && uid > max {
			max = uid
		}
	}
	return max + 1, nil
}

// AddUser 在 LDAP 中创建用户条目，包含以下 objectClass：
//   - inetOrgPerson：基本组织人员属性
//   - posixAccount：Linux UID/GID、家目录
//   - shadowAccount：密码属性（SSHA）
//   - sambaSamAccount：Samba 属性（NT hash、SID、flags）
//
// 参数：
//   - username：登录名，同时作为 uid、cn、sn
//   - uid：Linux UID 编号
//   - password：明文密码（内部计算 SSHA 和 NT hash）
//   - role：角色（"admin" 或 "user"），存入 employeeType 属性
func AddUser(conn *ldap.Conn, username string, uid int, password, role string) error {
	// 构建用户 DN：uid={username},ou=users,dc=nas,dc=local
	add := ldap.NewAddRequest(fmt.Sprintf("uid=%s,%s", username, UsersDN), nil)
	add.Attribute("objectClass", []string{"inetOrgPerson", "posixAccount", "shadowAccount", "sambaSamAccount"})
	add.Attribute("uid", []string{username})
	add.Attribute("cn", []string{username})
	add.Attribute("sn", []string{username})
	add.Attribute("uidNumber", []string{strconv.Itoa(uid)})
	add.Attribute("gidNumber", []string{"1000"})
	add.Attribute("homeDirectory", []string{"/data/" + username})
	add.Attribute("userPassword", []string{ssha(password)})
	add.Attribute("employeeType", []string{role}) // 角色存储：admin 或 user
	// Samba 属性：SID = {DomainSID}-{RID}，RID = uid*2 + 1000
	add.Attribute("sambaSID", []string{fmt.Sprintf("%s-%d", DomainSID, uid*2+1000)})
	add.Attribute("sambaNTPassword", []string{ntHash(password)})
	add.Attribute("sambaAcctFlags", []string{"[U          ]"}) // U = 普通用户账户
	add.Attribute("sambaPwdLastSet", []string{strconv.FormatInt(time.Now().Unix(), 10)})
	return conn.Add(add)
}

// GetUID 从 LDAP 查询用户的 uidNumber 和 gidNumber。
// 用户不存在时返回 uid=0, gid=1000。
func GetUID(conn *ldap.Conn, username string) (uid, gid int) {
	uid, gid = 0, 1000
	res, err := conn.Search(ldap.NewSearchRequest(
		UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, fmt.Sprintf("(uid=%s)", username), []string{"uidNumber", "gidNumber"}, nil,
	))
	if err != nil || len(res.Entries) == 0 {
		return
	}
	uid, _ = strconv.Atoi(res.Entries[0].GetAttributeValue("uidNumber"))
	gid, _ = strconv.Atoi(res.Entries[0].GetAttributeValue("gidNumber"))
	return
}

// GetUserRole 查询用户的 LDAP employeeType 属性，返回角色。
// 如果属性不存在或为空，默认返回 "user"（安全默认值）。
func GetUserRole(conn *ldap.Conn, username string) string {
	res, err := conn.Search(ldap.NewSearchRequest(
		UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, fmt.Sprintf("(uid=%s)", ldap.EscapeFilter(username)),
		[]string{"employeeType"}, nil,
	))
	if err != nil || len(res.Entries) == 0 {
		return "user" // 用户不存在或查询失败，保守返回普通用户角色
	}
	role := res.Entries[0].GetAttributeValue("employeeType")
	if role == "" {
		return "user"
	}
	return role
}

// UserInfo 表示从 LDAP 查询到的用户信息，供 API 响应使用。
type UserInfo struct {
	Username string `json:"username"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	Home     string `json:"home"`
	Role     string `json:"role"` // "admin" 或 "user"
}

// SearchUsers 搜索所有 posixAccount 用户，返回包含角色的用户列表。
func SearchUsers(conn *ldap.Conn) ([]UserInfo, error) {
	res, err := conn.Search(ldap.NewSearchRequest(
		UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=posixAccount)",
		[]string{"uid", "uidNumber", "gidNumber", "homeDirectory", "employeeType"}, nil,
	))
	if err != nil {
		return nil, err
	}
	users := make([]UserInfo, 0, len(res.Entries))
	for _, e := range res.Entries {
		uid, _ := strconv.Atoi(e.GetAttributeValue("uidNumber"))
		gid, _ := strconv.Atoi(e.GetAttributeValue("gidNumber"))
		role := e.GetAttributeValue("employeeType")
		if role == "" {
			role = "user" // 历史用户可能没有 employeeType，默认视为 user
		}
		users = append(users, UserInfo{
			Username: e.GetAttributeValue("uid"),
			UID:      uid,
			GID:      gid,
			Home:     e.GetAttributeValue("homeDirectory"),
			Role:     role,
		})
	}
	return users, nil
}

// CountUsers 返回 LDAP 中 posixAccount 用户总数。
// 用于注册时判断是否为第一个用户（count==0 → 自动设为 admin）。
func CountUsers(conn *ldap.Conn) (int, error) {
	res, err := conn.Search(ldap.NewSearchRequest(
		UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=posixAccount)",
		[]string{"uid"}, nil,
	))
	if err != nil {
		return 0, err
	}
	return len(res.Entries), nil
}

// DeleteUser 从 LDAP 删除指定用户的条目。
// 注意：此操作不可逆，删除前应确认用户存在。
func DeleteUser(conn *ldap.Conn, username string) error {
	dn := fmt.Sprintf("uid=%s,%s", ldap.EscapeFilter(username), UsersDN)
	return conn.Del(ldap.NewDelRequest(dn, nil))
}

// ssha 使用随机盐值计算 SSHA 密码哈希。
// 格式：{SSHA}base64(SHA1(password + salt) + salt)
// 这是 OpenLDAP 支持的密码存储方案之一。
func ssha(password string) string {
	salt := make([]byte, 8)
	rand.Read(salt)
	h := sha1.New()
	h.Write([]byte(password))
	h.Write(salt)
	return "{SSHA}" + base64.StdEncoding.EncodeToString(append(h.Sum(nil), salt...))
}

// ntHash 计算 Windows NT hash（MD4 of UTF-16LE）。
//
// 这是 Samba ldapsam 协议的强制要求：Samba 使用 NT hash 完成 NTLM 认证。
// MD4 在密码学上已被破解，但这是协议兼容性需求，不是安全选择。
// 密码仍由 SSHA 保护（用于 LDAP 绑定），NT hash 仅用于 SMB 协议兼容。
func ntHash(password string) string {
	// 将密码编码为 UTF-16LE
	encoded := utf16.Encode([]rune(password))
	b := make([]byte, len(encoded)*2)
	for i, r := range encoded {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	// MD4
	h := md4.New()
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

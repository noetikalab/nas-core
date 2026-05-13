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
	"golang.org/x/crypto/md4"
)

var (
	URL      string
	AdminDN  string
	AdminPW  string
	UsersDN  string
	DomainSID string
)

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

func Bind(username, password string) error {
	conn, err := ldap.DialURL(URL)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Bind(fmt.Sprintf("uid=%s,%s", username, UsersDN), password)
}

func NextUID(conn *ldap.Conn) (int, error) {
	res, err := conn.Search(ldap.NewSearchRequest(
		UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=posixAccount)", []string{"uidNumber"}, nil,
	))
	if err != nil {
		return 0, err
	}
	max := 1000
	for _, e := range res.Entries {
		if uid, err := strconv.Atoi(e.GetAttributeValue("uidNumber")); err == nil && uid > max {
			max = uid
		}
	}
	return max + 1, nil
}

func AddUser(conn *ldap.Conn, username string, uid int, password string) error {
	add := ldap.NewAddRequest(fmt.Sprintf("uid=%s,%s", username, UsersDN), nil)
	add.Attribute("objectClass", []string{"inetOrgPerson", "posixAccount", "shadowAccount", "sambaSamAccount"})
	add.Attribute("uid", []string{username})
	add.Attribute("cn", []string{username})
	add.Attribute("sn", []string{username})
	add.Attribute("uidNumber", []string{strconv.Itoa(uid)})
	add.Attribute("gidNumber", []string{"1000"})
	add.Attribute("homeDirectory", []string{"/data/" + username})
	add.Attribute("userPassword", []string{ssha(password)})
	add.Attribute("sambaSID", []string{fmt.Sprintf("%s-%d", DomainSID, uid*2+1000)})
	add.Attribute("sambaNTPassword", []string{ntHash(password)})
	add.Attribute("sambaAcctFlags", []string{"[U          ]"})
	add.Attribute("sambaPwdLastSet", []string{strconv.FormatInt(time.Now().Unix(), 10)})
	return conn.Add(add)
}

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

func ssha(password string) string {
	salt := make([]byte, 8)
	rand.Read(salt)
	h := sha1.New()
	h.Write([]byte(password))
	h.Write(salt)
	return "{SSHA}" + base64.StdEncoding.EncodeToString(append(h.Sum(nil), salt...))
}

// ntHash 计算 Windows NT hash（MD4 of UTF-16LE），Samba ldapsam 协议要求
func ntHash(password string) string {
	encoded := utf16.Encode([]rune(password))
	b := make([]byte, len(encoded)*2)
	for i, r := range encoded {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	h := md4.New()
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

package system

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
)

func CreateUser(username string, uid int) error {
	out, err := exec.Command("useradd", "-u", strconv.Itoa(uid), "-g", "1000", "-M", "-s", "/sbin/nologin", username).CombinedOutput()
	if err != nil {
		log.Printf("useradd failed for %s: %v: %s", username, err, out)
	}
	return err
}

func CreateDataDir(username string, uid int) {
	exec.Command("mkdir", "-p", "/data/"+username).Run()
	exec.Command("chown", fmt.Sprintf("%d:1000", uid), "/data/"+username).Run()
	exec.Command("chmod", "700", "/data/"+username).Run()
}

func SetACL(path, targetUser, perm string) {
	entry := fmt.Sprintf("user:%s:%s", targetUser, perm)
	exec.Command("chmod", "g+x", path).Run()
	exec.Command("setfacl", "-R", "-m", entry, path).Run()
	exec.Command("setfacl", "-R", "-d", "-m", entry, path).Run()
}

func RemoveACL(path, targetUser string) {
	exec.Command("setfacl", "-R", "-x", fmt.Sprintf("user:%s", targetUser), path).Run()
	exec.Command("setfacl", "-R", "-x", fmt.Sprintf("default:user:%s", targetUser), path).Run()
}

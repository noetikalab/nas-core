package handler

import (
	"net/http"
	"os/exec"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListServices 返回 SMB、NFS、WebDAV 三个服务的运行状态和监听端口。
//
// 检测逻辑：
//   - SMB：检查进程 smbd 是否存在 + 端口 445 是否监听
//   - NFS：检查进程 nfsd 是否存在 + 端口 2049 是否监听
//   - WebDAV：检查端口 8081 是否监听（由 Nginx dav-ext 模块提供，无独立进程名）
//
// 注意：进程检测使用 pgrep，容器环境中需要安装 procps 包。
func ListServices(c *gin.Context) {
	smb := checkService("smbd", []int{445})
	nfs := checkService("nfsd", []int{2049})
	webdav := webdavStatus()

	c.JSON(http.StatusOK, ServicesResponse{
		SMB:    smb,
		NFS:    nfs,
		WebDAV: webdav,
	})
}

// checkService 检查指定进程是否运行，以及其监听端口。
//
// 策略：先查进程（pgrep），进程存在再查端口（ss）。
// 端口和进程都正常才标记 Running=true 且 Port 非零。
// 进程存在但端口未监听 → Running=true 但 Port=0（服务可能正在启动）。
func checkService(procName string, ports []int) ServiceStatus {
	running := processRunning(procName)
	port := 0
	if running {
		// 从预设端口列表中找第一个正在监听的端口
		for _, p := range ports {
			if ok, _ := portListening(p); ok {
				port = p
				break
			}
		}
	}
	return ServiceStatus{Running: running, Port: port}
}

// processRunning 通过 pgrep -x 精确匹配进程名来判断进程是否在运行。
// -x 表示完整匹配，防止误匹配到包含该名称的长进程名。
func processRunning(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}

// portListening 使用 ss 命令检查指定端口是否有进程在监听。
//
// ss -tlnp 参数说明：
//   - -t：TCP
//   - -l：listening
//   - -n：numeric（不解析服务名）
//   - -p：显示进程
//   - sport = :{port}：按源端口过滤
func portListening(port int) (bool, error) {
	cmd := exec.Command("ss", "-tlnp", "sport", "= :"+strconv.Itoa(port))
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// webdavStatus 检查 WebDAV 服务状态。
// WebDAV 由 Nginx 在端口 8081 上提供，没有独立的进程名（nginx 也可能被其他服务使用），
// 因此只通过端口检测。
func webdavStatus() ServiceStatus {
	ok, _ := portListening(8081)
	return ServiceStatus{Running: ok, Port: 8081}
}

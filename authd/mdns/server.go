package mdns

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"nas/system"

	"github.com/grandcat/zeroconf"
)

// servers 保存所有已注册的 mDNS 服务实例。
// 每个物理接口和 P2P 虚拟接口各自注册一个独立的 service instance，
// 这样 APP 在所有接口上都能通过 mDNS 发现 NAS。
var servers []*zeroconf.Server

// deviceID 返回 NAS 设备标识，委托给 system.GetDeviceID()。
func deviceID() string {
	return system.GetDeviceID()
}

// serviceName 返回 mDNS 服务实例的基础名称。
func serviceName() string {
	return fmt.Sprintf("NAS-%s", deviceID())
}

// isPhysicalOrP2P 判断网卡名称是否应该在 mDNS 中广播。
// 允许物理接口（WiFi / 有线）和 WiFi Direct P2P 虚拟接口，
// 过滤掉 loopback、Docker 网桥等软件接口。
func isPhysicalOrP2P(name string) bool {
	return strings.HasPrefix(name, "wlp") ||   // WiFi 物理接口（如 wlp3s0）
		strings.HasPrefix(name, "wlan") ||     // WiFi 备选命名
		strings.HasPrefix(name, "eth") ||      // 有线网卡
		strings.HasPrefix(name, "enp") ||      // 有线网卡（Predictable Naming）
		strings.HasPrefix(name, "p2p-")        // WiFi Direct P2P 虚拟接口（如 p2p-wlp3s0-0）
}

// pickIPs 返回所有应该广播 mDNS 的 IPv4 地址。
// 原实现 pickIP() 只返回单个 IP，现改为返回所有符合条件的 IP，
// 使得 NAS 同时在 WiFi 局域网和 WiFi Direct P2P 上可被发现。
func pickIPs() []net.IP {
	var ips []net.IP
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		// 跳过未启用的接口和 loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// 只处理物理接口和 P2P 接口
		if !isPhysicalOrP2P(iface.Name) {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip := ipnet.IP.To4(); ip != nil && !ip.IsLoopback() {
					// 跳过 Docker bridge 子网（172.17.x.x ~ 172.31.x.x），
					// 避免广播容器内部 IP
					if ip[0] == 172 && (ip[1] >= 17 && ip[1] <= 31) {
						continue
					}
					ips = append(ips, ip)
				}
			}
		}
	}
	return ips
}

// Start 在所有合适的网络接口上启动 mDNS 广播。
// 每个 IP 注册一个独立 service instance，命名格式为 NAS-{deviceID}-{IP}。
// APP 通过 _nas._tcp 服务类型发现 NAS。
func Start(port int) error {
	ips := pickIPs()
	if len(ips) == 0 {
		log.Println("mDNS: 未找到可用的 IPv4 地址，跳过广播")
		return nil
	}

	host, _ := os.Hostname()
	txt := []string{
		"host=" + host,
		"version=1.0",
	}

	// 为每个 IP 注册独立的 mDNS service instance
	// 命名示例：NAS-zhangli-ASUS-TUF-192.168.49.1
	for _, ip := range ips {
		instance := fmt.Sprintf("%s-%s", serviceName(), ip.String())
		svr, err := zeroconf.RegisterProxy(
			instance,
			"_nas._tcp",
			"local.",
			port,
			host,
			[]string{ip.String()},
			txt,
			nil,
		)
		if err != nil {
			log.Printf("mDNS: 在 %s 上注册失败: %v", ip, err)
			continue
		}
		servers = append(servers, svr)
		log.Printf("mDNS 正在广播 %s（%s:%d）", instance, ip, port)
	}
	return nil
}

// Shutdown 停止所有 mDNS 广播。
func Shutdown() {
	for _, svr := range servers {
		if svr != nil {
			svr.Shutdown()
		}
	}
}

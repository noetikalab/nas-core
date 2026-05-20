package mdns

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/grandcat/zeroconf"
)

var server *zeroconf.Server

func deviceID() string {
	if id := os.Getenv("DEVICE_ID"); id != "" {
		return id
	}
	h, _ := os.Hostname()
	return h
}

func serviceName() string {
	return fmt.Sprintf("NAS-%s", deviceID())
}

// pickIP returns the first non-loopback, non-Docker IPv4 address.
func pickIP() net.IP {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip := ipnet.IP.To4(); ip != nil && !ip.IsLoopback() {
					// Skip Docker bridge subnets (172.17.x.x, 172.18.x.x, etc.)
					if ip[0] == 172 && (ip[1] >= 17 && ip[1] <= 31) {
						continue
					}
					return ip
				}
			}
		}
	}
	return nil
}

// Start advertises this NAS via mDNS on the LAN.
func Start(port int) error {
	ip := pickIP()
	if ip == nil {
		log.Println("mDNS: no suitable IPv4 address found, skipping")
		return nil
	}

	host, _ := os.Hostname()
	txt := []string{
		"host=" + host,
		"version=1.0",
	}

	var err error
	server, err = zeroconf.RegisterProxy(
		serviceName(),
		"_nas._tcp",
		"local.",
		port,
		host,
		[]string{ip.String()},
		txt,
		nil,
	)
	if err != nil {
		return fmt.Errorf("mdns: register: %w", err)
	}

	log.Printf("mDNS advertising %s on %s:%d", serviceName(), ip, port)
	return nil
}

// Shutdown stops the mDNS advertisement.
func Shutdown() {
	if server != nil {
		server.Shutdown()
	}
}

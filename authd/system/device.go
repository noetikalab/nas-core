// Package system 提供与宿主机系统交互的工具函数。
package system

import (
	"crypto/md5"
	"encoding/hex"
	"os"
)

// GetDeviceID 从宿主机 /etc/machine-id 推导 NAS 设备唯一标识。
// 返回值：/etc/machine-id 内容的 MD5 哈希前 12 位十六进制字符串。
// 例如 "b827eb3a1c2d"。
//
// 如果文件不可读（非宿主机环境），回退到 hostname。
// 该值用于 mDNS 服务名、/api/device-info 响应和 NFC 标签内容，
// 确保同一台 NAS 的所有入口返回一致的标识。
func GetDeviceID() string {
	data, err := os.ReadFile("/host/etc/machine-id")
	if err != nil {
		h, _ := os.Hostname()
		return h
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])[:12]
}

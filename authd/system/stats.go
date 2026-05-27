// Package system 提供系统级操作工具，包括用户管理、文件系统操作和系统指标采集。
package system

import (
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// GetCPUPercent 返回整体 CPU 使用率百分比（0-100）。
// 调用 cpu.Percent(0, false) 获取聚合值，非逐核数据。
func GetCPUPercent() (float64, error) {
	percents, err := cpu.Percent(0, false)
	if err != nil {
		return 0, err
	}
	if len(percents) == 0 {
		return 0, nil
	}
	return percents[0], nil
}

// GetMemory 返回虚拟内存统计信息，包括总量、已用、可用等。
func GetMemory() (*mem.VirtualMemoryStat, error) {
	return mem.VirtualMemory()
}

// GetDiskUsage 返回指定路径所在分区的磁盘使用情况。
// path 通常为 "/data"，即 NAS 数据存储挂载点。
func GetDiskUsage(path string) (*disk.UsageStat, error) {
	return disk.Usage(path)
}

// GetUptime 返回系统运行时长（秒）。
func GetUptime() (uint64, error) {
	up, err := host.Uptime()
	if err != nil {
		return 0, err
	}
	return up, nil
}

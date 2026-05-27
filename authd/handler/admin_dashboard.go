package handler

import (
	"net/http"
	"strconv"

	"nas/logbuf"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// DashboardStats
// @Summary      系统资源概览
// @Description  返回存储、CPU、内存使用率及运行时长，供管理后台首页统计卡片使用。
// @Tags         admin-dashboard
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} DashboardStatsResponse "系统资源数据"
// @Failure      401 {object} ErrorResponse "未登录或 token 过期"
// @Failure      500 {object} ErrorResponse "系统信息获取失败"
// @Router       /api/dashboard/stats [get]
func DashboardStats(c *gin.Context) {
	// 磁盘使用情况（/data 挂载点）
	diskUsage, err := system.GetDiskUsage("/data")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "disk stats failed"})
		return
	}

	// CPU 使用率（聚合值）
	cpuPct, err := system.GetCPUPercent()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "cpu stats failed"})
		return
	}

	// 内存使用情况
	memInfo, err := system.GetMemory()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "memory stats failed"})
		return
	}

	// 系统运行时长
	uptime, err := system.GetUptime()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "uptime failed"})
		return
	}

	c.JSON(http.StatusOK, DashboardStatsResponse{
		StorageUsed:  diskUsage.Used,
		StorageTotal: diskUsage.Total,
		CPUPercent:   cpuPct,
		MemUsed:      memInfo.Used,
		MemTotal:     memInfo.Total,
		Uptime:       uptime,
		DeviceCount:  1, // demo 阶段固定为 1，后续可从 mDNS 统计
	})
}

// RecentFiles
// @Summary      最近文件操作
// @Description  返回最近的文件操作记录（上传/下载/删除/新建/移动），供 Dashboard "最近操作" 列表使用。
// @Tags         admin-dashboard
// @Produce      json
// @Security     BearerAuth
// @Param        limit query int false "返回条数，默认 20，最大 100" default(20)
// @Success      200 {array} RecentEntry "操作记录列表"
// @Failure      401 {object} ErrorResponse "未登录或 token 过期"
// @Router       /api/dashboard/recent [get]
func RecentFiles(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	entries := logbuf.Default.Recent(limit)
	items := make([]RecentEntry, len(entries))
	for i, e := range entries {
		items[i] = RecentEntry{
			Name:   e.Path, // 文件名从路径提取，前端展示时处理
			Path:   e.Path,
			Action: e.Action,
			User:   e.User,
			Time:   e.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
			Size:   e.Size,
		}
	}
	c.JSON(http.StatusOK, items)
}

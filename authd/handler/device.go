package handler

import (
	"net/http"
	"os"

	"nas/system"

	"github.com/gin-gonic/gin"
)

func getDeviceID() string {
	return system.GetDeviceID()
}

func getHostname() string {
	h, _ := os.Hostname()
	return h
}

// DeviceInfo
// @Summary      Get NAS device information
// @Description  Return device ID, hostname, and version. Called by APP after mDNS discovery.
// @Tags         device
// @Produce      json
// @Success      200 {object} DeviceInfoResponse "Device information"
// @Router       /api/device-info [get]
func DeviceInfo(c *gin.Context) {
	c.JSON(http.StatusOK, DeviceInfoResponse{
		DeviceID: getDeviceID(),
		Hostname: getHostname(),
		Version:  "1.0",
	})
}

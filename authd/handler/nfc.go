// Package handler NFC 碰一碰登录与绑定接口。
//
// NFC 碰一碰流程：
//  1. APP 通过 NFC 读取 NAS 标签中的 device_id 和 IP
//  2. POST /api/nfc-login（phone_id 首次 → need_bind=true）
//  3. 提示用户输入密码 → POST /api/nfc-bind
//  4. 后续碰一碰直接免密登录
package handler

import (
	"net/http"

	"nas/ldap"
	jpkg "nas/pkg/jwt"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// NfcLogin NFC 碰一碰登录（公开接口，无需 JWT）。
//
// @Summary      NFC 碰一碰登录
// @Description  根据 phone_id 查找绑定用户。已绑定则签发 JWT，未绑定则返回 need_bind 引导 APP 跳转绑定页。
// @Tags         nfc
// @Accept       json
// @Produce      json
// @Param        body body NfcLoginRequest true "请求体"
// @Success      200 {object} NfcLoginResponse "已绑定返回 token，未绑定返回 need_bind"
// @Failure      400 {object} ErrorResponse "参数错误"
// @Failure      403 {object} ErrorResponse "device_id 不匹配"
// @Router       /api/nfc-login [post]
func NfcLogin(c *gin.Context) {
	var req NfcLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "参数错误"})
		return
	}
	if req.DeviceID != system.GetDeviceID() {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "设备不匹配"})
		return
	}

	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "服务不可用"})
		return
	}
	defer conn.Close()

	username, found := ldap.LookupPhoneID(conn, req.PhoneID)
	if !found {
		c.JSON(http.StatusOK, NfcLoginResponse{NeedBind: true})
		return
	}

	role := ldap.GetUserRole(conn, username)
	token, err := jpkg.Sign(username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "令牌签发失败"})
		return
	}
	c.JSON(http.StatusOK, NfcLoginResponse{Token: token, Username: username, Role: role})
}

// NfcBind NFC 首次绑定（公开接口，无需 JWT）。
//
// @Summary      NFC 首次绑定
// @Description  验证用户名密码后创建 phone_id ↔ username 绑定，后续碰一碰可免密登录。
// @Tags         nfc
// @Accept       json
// @Produce      json
// @Param        body body NfcBindRequest true "请求体"
// @Success      200 {object} NfcBindResponse "绑定成功，签发 JWT"
// @Failure      400 {object} ErrorResponse "参数错误"
// @Failure      401 {object} ErrorResponse "用户名或密码错误"
// @Failure      403 {object} ErrorResponse "device_id 不匹配"
// @Router       /api/nfc-bind [post]
func NfcBind(c *gin.Context) {
	var req NfcBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "参数错误"})
		return
	}
	if req.DeviceID != system.GetDeviceID() {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "设备不匹配"})
		return
	}

	// LDAP Bind 验证密码（不依赖主连接，独立连接验证）
	if err := ldap.Bind(req.Username, req.Password); err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "用户名或密码错误"})
		return
	}

	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "服务不可用"})
		return
	}
	defer conn.Close()

	if err := ldap.BindPhoneID(conn, req.PhoneID, req.Username); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "绑定失败"})
		return
	}

	role := ldap.GetUserRole(conn, req.Username)
	token, err := jpkg.Sign(req.Username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "令牌签发失败"})
		return
	}
	c.JSON(http.StatusOK, NfcBindResponse{Token: token, Username: req.Username, Role: role})
}

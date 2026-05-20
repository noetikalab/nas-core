package handler

import "nas/system"

type ErrorResponse struct {
	Error string `json:"error" example:"error description"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required" example:"alice"`
	Password string `json:"password" binding:"required,min=8" example:"12345678"`
}

type RegisterResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIs..."`
	UID   int    `json:"uid" example:"1001"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"alice"`
	Password string `json:"password" binding:"required" example:"12345678"`
}

type LoginResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIs..."`
}

type ValidateTokenResponse struct {
	Valid    bool   `json:"valid" example:"true"`
	Username string `json:"username" example:"alice"`
}

type VerifyPasswordRequest struct {
	Username string `json:"username" example:"alice"`
	Password string `json:"password" example:"12345678"`
}

type VerifyPasswordResponse struct {
	Success bool   `json:"success" example:"true"`
	UID     int    `json:"uid" example:"1001"`
	GID     int    `json:"gid" example:"1000"`
}

type SetPermissionRequest struct {
	Path       string `json:"path" binding:"required" example:"/data/alice"`
	TargetUser string `json:"target_user" binding:"required" example:"bob"`
	Action     string `json:"action" example:"readonly"`
	Readonly   bool   `json:"readonly"`
}

type OKResponse struct {
	OK bool `json:"ok" example:"true"`
}

type OKPathResponse struct {
	OK   bool   `json:"ok" example:"true"`
	Path string `json:"path" example:"/data/alice/photos"`
}

type MkdirRequest struct {
	Path string `json:"path" binding:"required" example:"/data/alice/photos"`
}

type MoveFileRequest struct {
	From string `json:"from" binding:"required" example:"/data/alice/old.txt"`
	To   string `json:"to" binding:"required" example:"/data/alice/new.txt"`
}

type ListFilesResponse struct {
	Path  string            `json:"path" example:"/data/alice"`
	Files []system.FileInfo `json:"files"`
}

type DeviceInfoResponse struct {
	DeviceID string `json:"device_id" example:"NAS-b827eb3a1c2d"`
	Hostname string `json:"hostname" example:"nas"`
	Version  string `json:"version" example:"1.0"`
}

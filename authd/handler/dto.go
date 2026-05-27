// Package handler 定义 HTTP API 的请求/响应结构体和 Gin handler 函数。
//
// 所有 DTO（Data Transfer Object）集中定义在此文件：
//   - 请求结构体：含 binding 标签（required、min 等），用于 Gin 自动校验
//   - 响应结构体：含 json tag 和 example tag（供 swaggo 生成 OpenAPI 文档）
//
// 命名约定：`{Action}Request` / `{Action}Response`，禁止匿名 struct。
package handler

import "nas/system"

// --- 通用 ---

type ErrorResponse struct {
	Error string `json:"error" example:"error description"`
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

// --- 认证 ---

type RegisterRequest struct {
	Username string `json:"username" binding:"required" example:"alice"`
	Password string `json:"password" binding:"required,min=8" example:"12345678"`
}

type RegisterResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIs..."`
	UID   int    `json:"uid" example:"1001"`
	Role  string `json:"role" example:"admin"` // 注册后的角色（首个用户为 admin）
}

type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"alice"`
	Password string `json:"password" binding:"required" example:"12345678"`
}

type LoginResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIs..."`
	Role  string `json:"role" example:"admin"` // 登录用户的角色，前端据此决定是否显示管理员菜单
}

type ValidateTokenResponse struct {
	Valid    bool   `json:"valid" example:"true"`
	Username string `json:"username" example:"alice"`
}

// --- 密码验证（内部接口） ---

type VerifyPasswordRequest struct {
	Username string `json:"username" example:"alice"`
	Password string `json:"password" example:"12345678"`
}

type VerifyPasswordResponse struct {
	Success bool `json:"success" example:"true"`
	UID     int  `json:"uid" example:"1001"`
	GID     int  `json:"gid" example:"1000"`
}

// --- 文件分享/权限 ---

type SetPermissionRequest struct {
	Path       string `json:"path" binding:"required" example:"/data/alice"`
	TargetUser string `json:"target_user" binding:"required" example:"bob"`
	Action     string `json:"action" example:"readonly"` // 权限操作："readonly"（只读）、"rw"（读写）、"remove"（移除）
	Readonly   bool   `json:"readonly"`                  // 兼容旧字段
}

// --- 文件操作 ---

type ListFilesResponse struct {
	Path  string            `json:"path" example:"/data/alice"`
	Files []system.FileInfo `json:"files"`
}

// --- 设备信息 ---

type DeviceInfoResponse struct {
	DeviceID string `json:"device_id" example:"NAS-b827eb3a1c2d"`
	Hostname string `json:"hostname" example:"nas"`
	Version  string `json:"version" example:"1.0"`
}

// --- Dashboard 管理后台 ---

// DashboardStatsResponse 系统资源概览，供管理后台首页统计卡片使用。
type DashboardStatsResponse struct {
	StorageUsed  uint64  `json:"storage_used" example:"2411724800"`  // 已用存储（字节）
	StorageTotal uint64  `json:"storage_total" example:"4000000000"` // 总存储（字节）
	CPUPercent   float64 `json:"cpu_percent" example:"45.2"`         // CPU 使用率百分比
	MemUsed      uint64  `json:"mem_used" example:"8589934592"`      // 已用内存（字节）
	MemTotal     uint64  `json:"mem_total" example:"16777216000"`    // 总内存（字节）
	Uptime       uint64  `json:"uptime" example:"86400"`             // 系统运行时长（秒）
	DeviceCount  int     `json:"device_count" example:"1"`           // 当前在线设备数
}

// RecentEntry Dashboard "最近操作" 列表中的单条记录。
type RecentEntry struct {
	Name   string `json:"name" example:"document.pdf"`           // 文件名（从 path 中提取）
	Path   string `json:"path" example:"/data/alice/"`           // 文件完整路径
	Action string `json:"action" example:"upload"`               // 操作类型
	User   string `json:"user" example:"alice"`                  // 操作者
	Time   string `json:"time" example:"2026-05-25T14:32:00Z"`   // 操作时间（RFC3339）
	Size   int64  `json:"size" example:"2411724"`                // 文件大小（字节）
}

// --- 用户管理 ---

// UserEntry 用户条目，供管理后台用户列表展示。
// Role 字段来自 LDAP employeeType 属性，决定用户是否有管理权限。
type UserEntry struct {
	Username string `json:"username" example:"alice"`
	UID      int    `json:"uid" example:"1001"`
	GID      int    `json:"gid" example:"1000"`
	Home     string `json:"home" example:"/data/alice"`
	Role     string `json:"role" example:"user"` // "admin" 或 "user"
}

// UserListResponse 用户列表 API 响应。
type UserListResponse struct {
	Users []UserEntry `json:"users"`
}

// --- 服务状态 ---

// ServiceStatus 单个服务的运行状态。
type ServiceStatus struct {
	Running bool `json:"running" example:"true"` // 是否正在运行
	Port    int  `json:"port" example:"445"`     // 监听的端口号
}

// ServicesResponse 所有网络服务的运行状态。
type ServicesResponse struct {
	SMB    ServiceStatus `json:"smb"`
	NFS    ServiceStatus `json:"nfs"`
	WebDAV ServiceStatus `json:"webdav"`
}

// --- 审计日志 ---

// LogEntry 审计日志条目，API 响应中不含内部 ID 和完整路径（由前端需求决定）。
type LogEntry struct {
	Timestamp string `json:"timestamp" example:"2026-05-25T14:32:00Z"` // ISO 8601 / RFC3339 格式
	Type      string `json:"type" example:"file"`                      // "file" | "auth" | "system"
	User      string `json:"user" example:"alice"`                     // 操作者用户名
	Action    string `json:"action" example:"upload"`                  // 操作名称
	Detail    string `json:"detail" example:"Uploaded 2.4 MB"`        // 补充说明
}

// LogListResponse 审计日志分页列表响应。
// TotalPages 由服务端计算：ceil(total / limit)。
type LogListResponse struct {
	Entries    []LogEntry `json:"entries"`                // 当前页日志条目
	Total      int64      `json:"total" example:"150"`    // 符合条件的总记录数
	Page       int        `json:"page" example:"1"`       // 当前页码
	Limit      int        `json:"limit" example:"20"`     // 每页条数
	TotalPages int        `json:"total_pages" example:"8"` // 总页数
}

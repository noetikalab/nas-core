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

// PermissionEntry 单条 ACL 权限条目，表示某个用户在某路径上的访问权限。
// 权限值来自 POSIX ACL 解析："r-x"=只读，"rwx"=读写。
type PermissionEntry struct {
	Username   string `json:"username" example:"bob"`
	Permission string `json:"permission" example:"r-x"` // "r-x"（只读）或 "rwx"（读写）
}

// PermissionListResponse 路径 ACL 查询响应的数据包装。
type PermissionListResponse struct {
	Path        string            `json:"path" example:"/data/shared/docs"`
	Permissions []PermissionEntry `json:"permissions"`
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

// CreateUserRequest admin 创建用户的请求体。密码最小 8 位。
// Role 默认 "user"，可指定 "admin"。
type CreateUserRequest struct {
	Username string `json:"username" binding:"required" example:"david"`
	Password string `json:"password" binding:"required,min=8" example:"12345678"`
	Role     string `json:"role" example:"user"` // 角色："admin" 或 "user"，默认 "user"
}

// CreateUserResponse admin 创建用户成功后的响应体。（不签发 JWT，与 RegisterResponse 不同）
type CreateUserResponse struct {
	OK       bool   `json:"ok" example:"true"`
	Username string `json:"username" example:"david"`
	UID      int    `json:"uid" example:"1004"`
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

// --- 审计日志 / 存证 ---

// CertifiedOperation 审计日志响应条目（对应 certified_operations 表全部字段）。
// 验证方通过此结构可独立还原操作场景：谁在什么时候对哪个文件做了什么。
type CertifiedOperation struct {
	ID        int64  `json:"id" example:"42"`
	Timestamp int64  `json:"timestamp" example:"1749300000000000000"` // Unix 纳秒
	Type      string `json:"type" example:"file"`                     // "file" | "auth" | "system"
	UserName  string `json:"user_name" example:"alice"`
	UserUID   int    `json:"user_uid" example:"1001"`
	Action    string `json:"action" example:"upload"`
	Path      string `json:"path" example:"/data/alice/contract.pdf"`
	DestPath  string `json:"dest_path,omitempty" example:""`            // move/rename 目标路径
	Detail    string `json:"detail,omitempty" example:""`
	FileName  string `json:"file_name" example:"contract.pdf"`
	IsDir     bool   `json:"is_dir" example:"false"`
	FileSize  int64  `json:"file_size" example:"2411724"`               // 字节
	MimeType  string `json:"mime_type" example:"application/pdf"`
	OwnerUID  int    `json:"owner_uid" example:"1001"`
	OwnerName string `json:"owner_name" example:"alice"`
	GroupName string `json:"group_name" example:"nas-users"`
	FilePerm  string `json:"file_perm" example:"rw-r--r--"`
	ModTime   int64  `json:"mod_time" example:"1749300000000000000"`    // Unix 纳秒
	FileHash  string `json:"file_hash,omitempty" example:"base64..."`   // base64 编码
	HashAlgo  string `json:"hash_algo" example:"SHA-256"`
}

// LogListResponse 审计日志分页列表响应。改用 CertifiedOperation 替代旧 LogEntry。
type LogListResponse struct {
	Entries    []CertifiedOperation `json:"entries"`
	Total      int64                `json:"total" example:"150"`
	Page       int                  `json:"page" example:"1"`
	Limit      int                  `json:"limit" example:"20"`
	TotalPages int                  `json:"total_pages" example:"8"`
}

// ProofRecordResponse 单条存证记录（API 响应，字段来自 proof_records 表）。
type ProofRecordResponse struct {
	CertID       int64  `json:"cert_id"`
	ChainIndex   int    `json:"chain_index"`
	PrevHash     string `json:"prev_hash,omitempty"`    // base64 编码
	DataHash     string `json:"data_hash"`              // base64 编码
	Signature    string `json:"signature,omitempty"`    // base64 编码，PUF 就绪前为空
	DeviceUID    string `json:"device_uid,omitempty"`
	SigTimestamp int64  `json:"sig_timestamp"`
	HashAlgo     string `json:"hash_algo"`
}

// ProofBundle 导出给验证方的完整存证包（含哈希链 + 操作记录 + 公钥）。
type ProofBundle struct {
	DeviceUID  string                `json:"device_uid"`
	PubKey     string                `json:"pub_key"`             // PUF 公钥，就绪前为空
	Records    []ProofRecordResponse `json:"records"`             // 哈希链记录
	Operations []CertifiedOperation  `json:"operations"`          // 对应的操作记录
	ExportTime int64                 `json:"export_time"`         // Unix 纳秒
	TotalCount int                   `json:"total_count"`
}

// ProofDetailResponse 单条日志 + 存证位置的组合查询响应。
type ProofDetailResponse struct {
	Operation   CertifiedOperation `json:"operation"`
	ProofRecord ProofRecordResponse `json:"proof_record"`
}

// --- NFC 碰一碰登录/绑定 ---

// NfcLoginRequest NFC 碰一碰登录请求。
// phone_id 由 APP 在首次启动时生成，永久不变，与手机硬件绑定。
type NfcLoginRequest struct {
	DeviceID string `json:"device_id" binding:"required" example:"b827eb3a1c2d"`
	PhoneID  string `json:"phone_id" binding:"required" example:"9774d56d682e549c"`
}

// NfcLoginResponse NFC 登录响应。
// 如果 phone_id 已绑定则返回 JWT，未绑定则 need_bind=true 引导 APP 跳转绑定页。
type NfcLoginResponse struct {
	Token    string `json:"token,omitempty" example:"eyJhbGci..."`
	Username string `json:"username,omitempty" example:"alice"`
	Role     string `json:"role,omitempty" example:"user"`
	NeedBind bool   `json:"need_bind,omitempty" example:"true"`
}

// NfcBindRequest NFC 首次绑定请求。
// 需要用户名密码验证，绑定后后续 NFC 碰一碰即可免密登录。
type NfcBindRequest struct {
	DeviceID string `json:"device_id" binding:"required" example:"b827eb3a1c2d"`
	PhoneID  string `json:"phone_id" binding:"required" example:"9774d56d682e549c"`
	Username string `json:"username" binding:"required" example:"alice"`
	Password string `json:"password" binding:"required,min=8" example:"12345678"`
}

// NfcBindResponse NFC 绑定成功响应，同时签发 JWT。
type NfcBindResponse struct {
	Token    string `json:"token" example:"eyJhbGci..."`
	Username string `json:"username" example:"alice"`
	Role     string `json:"role" example:"user"`
}

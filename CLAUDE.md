# CLAUDE.md

## 项目概述

NAS 多协议统一鉴权 Demo。验证 LDAP 作为唯一身份源，支持 HTTP/WebDAV（JWT）、SMB（ldapsam）、NFS（UID 映射）三协议认证，权限统一用 POSIX ACL 管理。完整产品方案包含 PUF 硬件身份接入与 SDK 扩展层。

## 关键决策

- **SMB 认证**：使用 `ldapsam` 后端，Samba 直接查 LDAP，不维护 tdbsam
- **权限真相**：POSIX ACL，管理后台调 `setfacl` 写入，三协议共用
- **SMB 权限预检问题**：授权时需同时 `chmod g+x` 目录，否则 Samba 在 POSIX ACL 之前就拒绝
- **NT Hash**：用 `golang.org/x/crypto/md4` 计算，MD4 是 SMB 协议要求，不是安全选择
- **authd 分包**：handler / ldap / pkg/jwt / system 四包，main.go 只做路由和中间件
- **文件操作 API**：在 authd 内实现 REST + JSON（不拆微服务），供 APP 使用；自定义响应字段，不走 WebDAV XML
- **WebDAV**：使用 Nginx + dav-ext 模块（端口 8081），通过 auth_request 调 authd `/api/validate-token` 鉴权，不用 Go 实现
- **API 前缀**：所有接口统一 `/api/` 前缀（含 login、register、files、device-info 等），仅 `/swagger/*any` 和 `/internal/*` 例外
- **文件操作路径范围**：JWT 中间件注入 username+role，admin 可操作 `/data/` 下任意目录，user 限制在 `/data/{username}/` 下
- **Swagger 文档**：swaggo 注解生成，Dockerfile 构建时自动 `swag init`，无需手动维护。访问 `/swagger/index.html`
- **DTO 命名类型**：`handler/dto.go` 存放所有请求/响应结构体，避免匿名 struct（供 swaggo 扫描 + 前端参考）
- **mDNS 局域网发现**：`mdns/server.go` 使用 grandcat/zeroconf RegisterProxy，pickIP() 过滤 Docker 网桥 IP（172.17-31.x.x），只广播物理网卡 WiFi IP。服务类型 `_nas._tcp`，服务名 `NAS-<device_id>`。配套 `/device-info` 接口供 APP 校验设备身份。

## 目录结构

| 路径 | 内容 |
|------|------|
| `authd/main.go` | 路由注册、JWT 中间件、全局 swaggo 注解、Swagger UI 路由 |
| `authd/handler/auth.go` | 注册/登录/验证 token/验密 |
| `authd/handler/file.go` | 文件操作：列表/上传/下载/建目录/删除/移动 |
| `authd/handler/permission.go` | ACL 权限设置 |
| `authd/handler/dto.go` | 所有请求/响应命名类型（14 个 struct） |
| `authd/ldap/` | LDAP 客户端：连接、AddUser、Bind、GetUID、NextUID |
| `authd/pkg/jwt/` | JWT Sign / Parse，Secret 由环境变量注入 |
| `authd/system/os.go` | useradd、mkdir、setfacl |
| `authd/system/file.go` | 文件系统操作：ListDir、OpenFile、WriteFile、ValidatePath |
| `authd/mdns/server.go` | mDNS 广播模块（grandcat/zeroconf），广播 _nas._tcp 服务 |
| `authd/handler/device.go` | GET /api/device-info（设备校验，无需 JWT） |
| `authd/docs/` | swag init 生成的 docs.go + swagger.json（编译进二进制） |
| `deploy/` | Dockerfile、smb.conf、nginx-webdav.conf、start.sh、ldap.conf、nsswitch.conf |
| `ldap/` | init.ldif（OU + 组初始化） |
| `docs/` | 设计文档（三个 MD） |

## 容器说明

| 容器 | 镜像 | 职责 |
|------|------|------|
| openldap | osixia/openldap:1.5.0 | 身份存储，启用 samba schema |
| ldap-init | osixia/openldap:1.5.0 | 一次性初始化 OU 和组，完成后退出 |
| nas | build: deploy/Dockerfile | Go authd + Nginx WebDAV + Samba + NFS |

## API 路由结构

所有接口统一 `/api/` 前缀，仅 Swagger 和内部回调例外：

| 分组 | 路由 | 认证 |
|------|------|------|
| 公开 | `/api/ping` `/api/device-info` `/api/register` `/api/login` | 无 |
| 认证 | `/api/validate-token` `/api/share/permission` | JWT |
| 文件 | `/api/files` `/api/files/download` `/api/files/upload` `/api/files/mkdir` `/api/files/move` | JWT（角色自适应） |
| 管理 | `/api/dashboard/*` `/api/users/*` `/api/logs` `/api/services` | JWT + admin |
| 其他 | `/swagger/*any` `/internal/verify-password` | 无 |

## 开发注意事项

- `ldap-init` 的 entrypoint 必须用列表形式覆盖，不能用 `command: >` 或 `command: |`（会被 osixia entrypoint 拦截）
- `sambaSamAccount` objectClass 需要 OpenLDAP 加载 samba schema（`LDAP_EXTRA_SCHEMAS: "samba"`）
- NFS 在容器里需要 `privileged: true`
- Go 模块名为 `nas`，包引用路径如 `nas/handler`、`nas/ldap`、`nas/pkg/jwt`、`nas/system`
- Swagger 文档由 Dockerfile 构建阶段自动生成（`swag init`），无需本地手动运行
- swag CLI 版本必须与 go.mod 中 swaggo/swag 库版本一致（当前 v1.8.12），否则生成的 docs.go 字段不兼容
- 新增/修改 handler 后，在函数上方按现有格式添加 swaggo 注解（`@Summary` / `@Tags` / `@Param` / `@Success` / `@Failure` / `@Router`）
- 请求/响应结构体统一定义在 `handler/dto.go`，使用命名导出类型（`error` tag 写示例值）
- 端口分配：`8080`=authd HTTP API，`8081`=Nginx WebDAV，`445`=SMB，`2049`=NFS

## 常用命令

```bash
# 重建
sudo docker compose down -v && sudo docker compose up --build -d

# 查看 LDAP 用户
sudo docker exec ldap-demo-openldap-1 ldapsearch \
  -x -H ldap://localhost \
  -D "cn=admin,dc=nas,dc=local" -w admin123 \
  -b "ou=users,dc=nas,dc=local"

# 查看 Samba 用户
sudo docker exec ldap-demo-nas-1 pdbedit -L

# 查看 authd 日志
sudo docker compose logs nas -f

# 访问 Swagger UI
# 浏览器打开 http://<host-ip>:8080/swagger/index.html
```

## 参考文档

- [技术架构设计文档 v2（飞书）](https://my.feishu.cn/docx/EhQodDF20oHLMixoRaWcrejinIf) — 完整架构图、PUF 接入方案、SDK 设计

## 踩坑记录

### Docker 构建：golang 镜像拉取失败

**现象**：`docker compose up --build` 时 `failed to fetch content descriptor ... from remote: not found`

**根因**：Docker Hub 部分 layer blob 在国内网络环境下偶发拉取失败（与 node:24-alpine 同理）

**处理**：
1. 先尝试 `docker pull golang:1.25` 单独拉取镜像（通常能解决瞬时网络问题，当前版本存在且可用）
2. 如持续失败，才考虑降级镜像版本

**注意**：`golang.org/x/sync@v0.20.0` 等间接依赖需要 `go >= 1.25.0`，因此 Dockerfile 必须使用 `golang:1.25`（golang:1.24 无法编译）

## 约束

- **Go 版本**：Dockerfile 固定 `golang:1.25`（依赖要求 go >= 1.25）
- **go.mod**：go 指令需 `1.25.0`，本地 Go 1.26 编译和 go mod tidy 都兼容

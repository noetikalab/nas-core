# ldap-demo

NAS 多协议统一鉴权与权限管理 Demo，验证 LDAP 作为唯一身份数据源，支持 HTTP/WebDAV、SMB、NFS 三协议统一认证与权限控制。完整产品方案包含 PUF 硬件身份接入与 SDK 扩展。

## 架构

```
APP (React Native)
  ├─ mDNS 发现 ←── NAS 广播 _nas._tcp（局域网）
  ├─ REST JSON ──→ authd (Go) :8080 ──→ LDAP ──→ JWT
  │                ├─ /device-info (设备校验)
  │                ├─ /files/*     (文件操作)
  │                ├─ /share/*     (权限管理)
  │                └─ /swagger/*   (API 文档)
  │
PC 文件管理器
  └─ WebDAV ────→ Nginx :8081 ──→ authd /validate-token ──→ /data

SMB ────────────→ Samba (ldapsam) :445 ──→ LDAP
NFS ────────────→ 内核 UID 映射 :2049

文件权限：POSIX ACL（唯一权限真相，所有协议共用）
```

> **注意**：nas 容器使用 `network_mode: host`，直接使用宿主机网络栈，目的是让 mDNS UDP 多播包能到达物理局域网。host 模式下容器的所有端口直接暴露在宿主机上。
## 目录结构

```
ldap-demo/
├── docker-compose.yml          # 三容器编排
├── authd/                      # Go 认证服务源码
│   ├── main.go                 # 路由 + JWT 中间件
│   ├── handler/
│   │   ├── auth.go             # Register / Login / ValidateToken / VerifyPassword
│   │   ├── file.go             # ListFiles / UploadFile / DownloadFile / Mkdir / DeleteFile / MoveFile
│   │   ├── permission.go       # SetPermission (setfacl)
│   │   └── dto.go              # 所有请求/响应命名类型（14 个 struct）
│   ├── ldap/
│   │   └── client.go           # LDAP 连接、AddUser、Bind、GetUID
│   ├── pkg/jwt/
│   │   └── jwt.go              # JWT Sign / Parse
│   ├── system/
│   │   ├── os.go               # CreateUser / CreateDataDir / SetACL
│   │   └── file.go             # ListDir / OpenFile / WriteFile / ValidatePath
│   └── docs/                   # swag init 生成（docs.go + swagger.json）
│   ├── go.mod
│   └── go.sum
├── deploy/                     # 容器运行时配置
│   ├── Dockerfile              # 多阶段构建（Go + swag init + Nginx WebDAV）
│   ├── smb.conf                # Samba 配置（ldapsam 后端）
│   ├── nginx-webdav.conf       # Nginx WebDAV 配置（auth_request → authd）
│   ├── start.sh                # 容器启动脚本
│   ├── ldap.conf               # LDAP 客户端配置
│   └── nsswitch.conf           # NSS 配置
├── ldap/
│   └── init.ldif               # LDAP 初始化数据（OU + 组）
├── logs/                       # 测试日志
└── docs/
    ├── 01-auth-and-permission-design.md   # 鉴权与权限管理方案
    ├── 02-demo-validation-summary.md      # Demo 验证总结
    └── 03-permission-acl-analysis.md      # 权限缺陷分析
```

## 快速启动

```bash
docker compose down -v
docker compose up --build -d
docker compose logs ldap-init   # 确认看到 "ldap init done"
```

## 端口分配

| 端口 | 服务 | 用途 |
|------|------|------|
| `8080` | authd (Go) | REST API + Swagger UI |
| `8081` | Nginx WebDAV | PC 文件管理器挂载（PROPFIND/PUT/GET） |
| `445` | Samba (SMB) | Windows/SMB 文件共享 |
| `2049` | NFS | NFS 挂载 |

## API

### 公开接口（无需认证）

| 接口 | 说明 |
|------|------|
| `GET /device-info` | 设备信息校验（APP mDNS 发现后确认设备身份） |
| `GET /ping` | 连通性测试 |
| `POST /register` | 注册（写 LDAP + 创建系统用户） |
| `POST /login` | 登录（LDAP Bind 验证，返回 JWT） |
| `POST /internal/verify-password` | 内部接口，供 PAM 调用 |
| `GET /swagger/*any` | Swagger UI（浏览器打开查看交互式文档） |

### 需 JWT 认证（Authorization: Bearer \<token\>）

| 接口 | 说明 |
|------|------|
| `GET /validate-token` | 验证 JWT 是否有效 |
| `POST /share/permission` | 设置文件 ACL 权限，action: readonly / readwrite / remove |
| `GET /files?path=` | 列目录，返回 JSON（name/size/type/modified/permission） |
| `GET /files/download?path=` | 下载文件（二进制流） |
| `POST /files/upload` | 上传文件（multipart/form-data，字段 file + path） |
| `POST /files/mkdir` | 创建目录 `{"path":"..."}` |
| `DELETE /files?path=` | 删除文件或目录 |
| `POST /files/move` | 移动/重命名 `{"from":"...","to":"..."}` |

## mDNS 局域网发现

NAS 启动后通过 mDNS（Multicast DNS）在局域网广播自身，APP 端使用 Android `NsdManager` 发现设备。

### NAS 广播内容

| 字段 | 值 | 说明 |
|------|-----|------|
| 服务类型 | `_nas._tcp` | 固定值 |
| 服务名 | `NAS-<device_id>` | 如 `NAS-b827eb3a1c2d` |
| 端口 | `8080` | authd HTTP API |
| TXT 记录 | `host=<hostname>`, `version=1.0` | 附加信息 |

### 实现

代码在 `authd/mdns/server.go`，使用 `github.com/hashicorp/mdns` 库（纯 Go 实现，无需外部 avahi-daemon）。在 `main.go` 中以 goroutine 启动：

```go
go func() {
    if err := mdns.Start(8080); err != nil {
        log.Printf("mDNS: %v", err)
    }
}()
```

### APP 发现流程

```
1. APP 打开 Discovery 页面 → NsdManager.discoverServices("_nas._tcp")
2. NAS 响应 → 返回 { name, ip, port }
3. APP 点击设备 → GET /device-info → 校验 device_id 匹配
4. 校验通过 → 存 baseUrl → 跳 Login
```

### 网络要求

- **`network_mode: host`** 是必须的：Docker 默认 bridge 网络会阻断 UDP 多播包（224.0.0.251:5353），host 模式让多播直接走物理网卡
- NAS 宿主机和手机必须连接**同一局域网**（同一 WiFi 或同一交换机）
- USB 网络共享理论上在同一子网，但部分 Android 厂商不路由多播包，建议用 WiFi 测试

## Swagger 文档

访问 `http://<host-ip>:8080/swagger/index.html` 查看交互式 API 文档。

- 所有接口按分组展示（auth / files / share）
- 点击右上角 **Authorize**，输入 `Bearer <JWT token>` 即可在页面上直接调接口
- 文件上传接口有文件选择器，下载接口有下载链接
- swagger.json 地址：`http://<host-ip>:8080/swagger/doc.json`（可导入 Postman / Apifox）
- 文档由 Dockerfile 构建时自动生成，修改接口后只需 `docker compose up --build -d`

## WebDAV

Nginx 在 8081 端口提供 WebDAV 服务，供 PC 文件管理器（Windows 映射网络驱动器 / Mac Finder 连接服务器）挂载。

- 鉴权：每次请求 Nginx 先调 `authd:8080/validate-token` 验证 JWT
- 方式：客户端在 HTTP 头带上 `Authorization: Bearer <JWT>`
- 支持方法：PROPFIND / PUT / GET / MKCOL / DELETE / COPY / MOVE

示例（curl）：
```bash
# 列目录
curl -X PROPFIND http://localhost:8081/data/alice/ \
  -H "Authorization: Bearer $TOKEN"

# 上传文件
curl -T ./photo.jpg http://localhost:8081/data/alice/photo.jpg \
  -H "Authorization: Bearer $TOKEN"
```

## 验证步骤

```bash
# 注册
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"pass1234"}'

curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"username":"bob","password":"pass5678"}'

# 登录，获取 token
TOKEN=$(curl -s -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"pass1234"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

# 验证 token
curl http://localhost:8080/validate-token -H "Authorization: Bearer $TOKEN"

# SMB 访问
smbclient //localhost/data -U alice%pass1234 -c "ls"

# 权限验证：alice 创建文件，bob 默认无权访问
sudo docker exec ldap-demo-nas-1 bash -c "echo hello > /data/alice/test.txt"
smbclient //localhost/data -U bob%pass5678 -c "cd alice;ls"   # ACCESS_DENIED

# 授权 bob 只读
curl -X POST http://localhost:8080/share/permission \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"path":"/data/alice","target_user":"bob","action":"readonly"}'
smbclient //localhost/data -U bob%pass5678 -c "cd alice;ls"              # 成功
smbclient //localhost/data -U bob%pass5678 -c "put /etc/hostname alice\hack.txt"  # 失败

# 升级为读写
curl -X POST http://localhost:8080/share/permission \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"path":"/data/alice","target_user":"bob","action":"readwrite"}'
smbclient //localhost/data -U bob%pass5678 -c "put /etc/hostname alice\hack.txt"  # 成功

# 撤销授权
curl -X POST http://localhost:8080/share/permission \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"path":"/data/alice","target_user":"bob","action":"remove"}'
smbclient //localhost/data -U bob%pass5678 -c "cd alice;ls"   # ACCESS_DENIED

# 查看 ACL
sudo docker exec ldap-demo-nas-1 getfacl /data/alice

# 文件操作 API
curl http://localhost:8080/files -H "Authorization: Bearer $TOKEN"
curl -X POST http://localhost:8080/files/mkdir \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"path":"/data/alice/photos"}'

# 访问 Swagger UI
# 浏览器打开 http://localhost:8080/swagger/index.html
# 右上角 Authorize → 输入 Bearer <token> → 所有接口可在页面直接调用
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `JWT_SECRET` | JWT 签名密钥 |
| `LDAP_URL` | LDAP 服务地址。host 模式下用 `ldap://127.0.0.1:389`，bridge 模式下用 `ldap://openldap:389` |
| `LDAP_ADMIN_DN` | 管理员 DN，如 `cn=admin,dc=nas,dc=local` |
| `LDAP_ADMIN_PW` | 管理员密码 |
| `DEVICE_ID` | （可选）覆盖 mDNS 广播和 `/device-info` 中的设备标识，默认取 OS hostname |

## PUF 扩展方案

完整产品方案在现有基础上叠加 PUF 硬件身份层：

- **设备绑定**：PUF 芯片为每台 NAS 生成唯一硬件指纹，防克隆
- **双因素认证**：LDAP 用户身份 + PUF 设备证明
- **司法存证**：PUF 签名 + 时间戳 + 哈希链，文件可验证溯源
- **SDK 开放**：封装 REST API，支持第三方应用集成

详见[技术架构设计文档（飞书）](https://my.feishu.cn/docx/EhQodDF20oHLMixoRaWcrejinIf)。

## 文档

- [鉴权与权限管理方案](docs/01-auth-and-permission-design.md)
- [Demo 验证总结](docs/02-demo-validation-summary.md)
- [权限缺陷分析](docs/03-permission-acl-analysis.md)
- [技术架构设计文档 v2（飞书）](https://my.feishu.cn/docx/EhQodDF20oHLMixoRaWcrejinIf)

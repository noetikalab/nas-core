<h1 align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/Docker-✓-2496ED?style=flat&logo=docker" alt="Docker">
  <img src="https://img.shields.io/badge/LDAP-✓-0C63AE?style=flat" alt="LDAP">
  <img src="https://img.shields.io/badge/license-MIT-blue?style=flat" alt="License">
</h1>

<h1 align="center">NAS Core</h1>
<h3 align="center">可信 NAS 多协议统一鉴权与 PUF 存证引擎</h3>

<p align="center">基于 LDAP 的 HTTP/WebDAV/SMB/NFS 四协议统一认证 · POSIX ACL 权限控制 · PUF 硬件存证</p>

---

## ✨ 特性

| 能力 | 说明 |
|------|------|
| 🔐 **多协议统一认证** | HTTP (JWT) / WebDAV (Nginx auth_request) / SMB (ldapsam) / NFS (UID 映射)，LDAP 唯一身份源 |
| 🗂️ **文件管理 API** | RESTful 接口：列表、上传、下载、新建目录、删除、移动/重命名；admin/user 角色自适应路径 |
| 🔒 **POSIX ACL 权限** | 三协议共用同一套权限体系，setfacl 统一管理；支持只读/读写/撤销 |
| 📡 **mDNS 局域网发现** | 自动广播 `_nas._tcp` 服务，Android/iOS 免配置发现 NAS |
| 📻 **WiFi Direct P2P** | 无路由器场景下手机直连 NAS，NAS 作为 Group Owner，固定 IP `192.168.49.1` |
| 👥 **用户管理** | admin 创建/删除用户；普通用户个人目录 + `/data/shared` 共享目录 |
| 🗃️ **PUF 存证** | 每个文件操作记录完整元信息 + SHA-256 指纹 → 哈希链 → PUF SM2 签名（就绪后） |
| 📋 **审计日志** | 22 字段完整快照：操作者、文件元信息、内容指纹、哈希链位置；可导出 ProofBundle |
| 📖 **Swagger 文档** | 构建时自动生成，浏览器交互式调试 |

---

## 🏗️ 架构

```
┌──────────────────────────────────────────────────────────────┐
│                     NAS 容器 (network_mode: host)              │
│                                                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                      authd :8080                        │ │
│  │  Go · Gin · JWT · Swagger · mDNS · P2P                  │ │
│  │  ┌────────┐ ┌──────────┐ ┌──────────┐ ┌─────────────┐  │ │
│  │  │ Auth   │ │  Files   │ │  Share   │ │  Dashboard  │  │ │
│  │  │ Register│ │  List    │ │ SetACL   │ │  Stats      │  │ │
│  │  │ Login   │ │  Upload  │ │ QueryACL │ │  Users      │  │ │
│  │  │ JWT     │ │  Download│ │          │ │  Logs       │  │ │
│  │  └────────┘ └──────────┘ └──────────┘ └─────────────┘  │ │
│  └───────────────────────┬─────────────────────────────────┘ │
│                          │                                   │
│  ┌──────────┐ ┌──────────┴──────────┐ ┌────────────────────┐ │
│  │ Nginx    │ │    Samba + NFS      │ │  WiFi Direct P2P   │ │
│  │ WebDAV   │ │    :445   :2049     │ │  GO: 192.168.49.1  │ │
│  │ :8081    │ │    ldapsam / UID    │ │                    │ │
│  └──────────┘ └─────────────────────┘ └────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
         │                                 │
    ┌────▼────┐                      ┌─────▼─────┐
    │OpenLDAP │                      │  SQLite   │
    │ :389    │                      │  .nas.db  │
    │身份存储  │                      │ 存证 + 日志 │
    └─────────┘                      └───────────┘
```

---

## 🚀 快速启动

```bash
git clone https://github.com/noetikalab/nas-core.git
cd nas-core
sudo docker compose up --build -d
# → Swagger UI: http://localhost:8080/swagger/index.html
```

---



## 📋 API 全览

### 公开接口

| 方法 | 端点 | 说明 |
|------|------|------|
| `GET` | `/api/ping` | 健康检查 |
| `GET` | `/api/device-info` | 设备身份（mDNS 发现后校验） |
| `POST` | `/api/register` | 注册新用户 |
| `POST` | `/api/login` | 登录，返回 JWT |

### 文件操作（JWT）

| 方法 | 端点 | 说明 |
|------|------|------|
| `GET` | `/api/files?path=` | 列出目录内容 |
| `GET` | `/api/files/download?path=` | 下载文件 |
| `POST` | `/api/files/upload` | 上传文件（multipart） |
| `POST` | `/api/files/mkdir` | 新建目录 |
| `DELETE` | `/api/files?path=` | 删除文件/目录 |
| `POST` | `/api/files/move` | 移动/重命名 |

### 权限管理（JWT）

| 方法 | 端点 | 说明 |
|------|------|------|
| `POST` | `/api/share/permission` | 设置 ACL（readonly/rw/remove） |
| `GET` | `/api/share/permissions?path=` | 查询路径上的 ACL 权限列表 |

### 管理接口（JWT + admin）

| 方法 | 端点 | 说明 |
|------|------|------|
| `GET` | `/api/dashboard/stats` | 系统资源概览 |
| `GET` | `/api/dashboard/recent` | 最近文件操作 |
| `GET` | `/api/users` | 用户列表 |
| `POST` | `/api/users` | admin 创建用户 |
| `DELETE` | `/api/users/:username` | 删除用户 |
| `GET` | `/api/logs` | 审计日志（分页） |
| `GET` | `/api/proof/:id` | 存证记录详情 |
| `GET` | `/api/proof/bundle` | 导出 ProofBundle 存证包 |

---

## 🔒 PUF 存证

每个文件操作生成不可否认的存证记录：

```
文件操作发生
    │
    ▼
certified_operations 表          proof_records 表
══════════════════════            ═══════════════
· 操作者（用户 + UID）             · chain_index 链表序号
· 操作类型（upload/delete/move）    · prev_hash   前一节点哈希
· 目标路径                        · data_hash   SM3(操作+pre_hash)
· 文件元信息（大小/MIME/权限/时间）  · signature   PUF SM2 签名
· 文件哈希（SHA-256）              · device_uid  PUF 硬件 ID
```

验证方拿到导出的 ProofBundle + PUF 公钥 → 验证哈希链完整性 → SM2 验签 → 不可否认。详见 `CLAUDE.md`。

---

## 📡 mDNS 发现

| 字段 | 值 |
|------|-----|
| 服务类型 | `_nas._tcp` |
| 服务名 | `NAS-{deviceID}-{IP}` |
| 端口 | `8080` |

在多接口上同时广播（物理网卡 + P2P 虚拟接口），APP 使用 `NsdManager.discoverServices("_nas._tcp")` 发现。

---

## 📂 项目结构

```
├── authd/                     # Go 服务源码
│   ├── main.go                # 路由 + 中间件
│   ├── handler/               # 认证 / 文件 / 权限 / 仪表盘 / 用户 / 存证
│   ├── ldap/client.go         # LDAP 客户端
│   ├── pkg/jwt/               # JWT 签发与验证
│   ├── system/                # Linux 系统操作（useradd/mkdir/setfacl）
│   ├── mdns/server.go         # mDNS 广播
│   ├── db/migrations/         # SQLite 嵌入式迁移
│   └── repository/            # SQLite 仓库（日志 + 存证）
├── deploy/                    # Docker 配置
│   ├── Dockerfile             # 多阶段构建
│   ├── start.sh               # 容器启动脚本（含 P2P）
│   ├── smb.conf               # Samba 配置
│   └── nginx-webdav.conf      # WebDAV 反代配置
├── docs/                      # 设计文档
└── docker-compose.yml         # 三容器编排
```

---

## 🧪 验证

```bash
# 注册并登录
TOKEN=$(curl -s -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"pass1234"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

# 文件操作
curl http://localhost:8080/api/files -H "Authorization: Bearer $TOKEN"
curl -X POST http://localhost:8080/api/files/mkdir \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"path":"/data/alice/photos"}'

# 权限管理
curl -X POST http://localhost:8080/api/share/permission \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"path":"/data/alice","target_user":"bob","action":"readonly"}'

# Swagger UI
open http://localhost:8080/swagger/index.html
```

---

## 📚 文档

| 文档 | 说明 |
|------|------|
| [CLAUDE.md](CLAUDE.md) | AI/同事接手指引 — 关键决策、架构、约束、踩坑记录 |
| [设计文档](docs/) | 鉴权设计、验证总结、权限分析 |
| [Wiki](https://github.com/noetikalab/nas-core/wiki) | WiFi P2P + NFC + APP 综合方案 |

---

## 🔗 相关项目

| 项目 | 说明 |
|------|------|
| [nas-web](https://github.com/noetikalab/nas-web) | 管理后台（Next.js） |
| NasApp | Android 客户端（React Native） |

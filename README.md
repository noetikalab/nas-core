# ldap-demo

NAS 多协议统一鉴权与权限管理 Demo，验证 LDAP 作为唯一身份数据源，支持 HTTP/WebDAV、SMB、NFS 三协议统一认证与权限控制。完整产品方案包含 PUF 硬件身份接入与 SDK 扩展。

## 架构

```
客户端
  ├─ HTTP/WebDAV ──→ authd (Go) ──→ LDAP Bind 验证 ──→ JWT
  ├─ SMB ──────────→ Samba (ldapsam) ──→ LDAP Bind 验证
  └─ NFS ──────────→ 内核 UID 映射（生产需 Kerberos）

文件权限：POSIX ACL（唯一权限真相，三协议共用）

PUF 扩展：PUF芯片 → PUF Agent → authd（设备身份绑定 + 存证签名）
```

## 目录结构

```
ldap-demo/
├── docker-compose.yml          # 三容器编排
├── authd/                      # Go 认证服务源码
│   ├── main.go                 # 路由 + JWT 中间件
│   ├── handler/
│   │   ├── auth.go             # Register / Login / ValidateToken / VerifyPassword
│   │   └── permission.go       # SetPermission (setfacl)
│   ├── ldap/
│   │   └── client.go           # LDAP 连接、AddUser、Bind、GetUID
│   ├── pkg/jwt/
│   │   └── jwt.go              # JWT Sign / Parse
│   ├── system/
│   │   └── os.go               # CreateUser / CreateDataDir / SetACL
│   ├── go.mod
│   └── go.sum
├── deploy/                     # 容器运行时配置
│   ├── Dockerfile
│   ├── smb.conf                # Samba 配置（ldapsam 后端）
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

## API

| 接口 | 说明 |
|------|------|
| `POST /register` | 注册（写 LDAP + 创建系统用户） |
| `POST /login` | 登录（LDAP Bind 验证，返回 JWT） |
| `GET /validate-token` | 验证 JWT（供 WebDAV nginx auth_request 使用） |
| `POST /share/permission` | 设置文件 ACL 权限（需 JWT），支持 readonly/readwrite/remove |
| `POST /internal/verify-password` | 内部接口，供 PAM 调用 |

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
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `JWT_SECRET` | JWT 签名密钥 |
| `LDAP_URL` | LDAP 服务地址，如 `ldap://openldap:389` |
| `LDAP_ADMIN_DN` | 管理员 DN，如 `cn=admin,dc=nas,dc=local` |
| `LDAP_ADMIN_PW` | 管理员密码 |

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

# CLAUDE.md

## 项目概述

NAS 多协议统一鉴权 Demo。验证 LDAP 作为唯一身份源，支持 HTTP/WebDAV（JWT）、SMB（ldapsam）、NFS（UID 映射）三协议认证，权限统一用 POSIX ACL 管理。完整产品方案包含 PUF 硬件身份接入与 SDK 扩展层。

## 关键决策

- **SMB 认证**：使用 `ldapsam` 后端，Samba 直接查 LDAP，不维护 tdbsam
- **权限真相**：POSIX ACL，管理后台调 `setfacl` 写入，三协议共用
- **SMB 权限预检问题**：授权时需同时 `chmod g+x` 目录，否则 Samba 在 POSIX ACL 之前就拒绝
- **NT Hash**：用 `golang.org/x/crypto/md4` 计算，MD4 是 SMB 协议要求，不是安全选择
- **authd 分包**：handler / ldap / pkg/jwt / system 四包，main.go 只做路由和中间件

## 目录结构

| 路径 | 内容 |
|------|------|
| `authd/main.go` | 路由注册、JWT 中间件 |
| `authd/handler/` | HTTP handler：auth.go（注册/登录/验证）、permission.go（ACL） |
| `authd/ldap/` | LDAP 客户端：连接、AddUser、Bind、GetUID、NextUID |
| `authd/pkg/jwt/` | JWT Sign / Parse，Secret 由环境变量注入 |
| `authd/system/` | OS 操作：useradd、mkdir、setfacl |
| `deploy/` | Dockerfile、smb.conf、start.sh、ldap.conf、nsswitch.conf |
| `ldap/` | init.ldif（OU + 组初始化） |
| `docs/` | 设计文档（三个 MD） |

## 容器说明

| 容器 | 镜像 | 职责 |
|------|------|------|
| openldap | osixia/openldap:1.5.0 | 身份存储，启用 samba schema |
| ldap-init | osixia/openldap:1.5.0 | 一次性初始化 OU 和组，完成后退出 |
| nas | build: deploy/Dockerfile | Go authd + Samba + NFS |

## 开发注意事项

- `ldap-init` 的 entrypoint 必须用列表形式覆盖，不能用 `command: >` 或 `command: |`（会被 osixia entrypoint 拦截）
- `sambaSamAccount` objectClass 需要 OpenLDAP 加载 samba schema（`LDAP_EXTRA_SCHEMAS: "samba"`）
- NFS 在容器里需要 `privileged: true`
- Go 模块名为 `nas`，包引用路径如 `nas/handler`、`nas/ldap`、`nas/pkg/jwt`、`nas/system`

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
```

## 参考文档

- [技术架构设计文档 v2（飞书）](https://my.feishu.cn/docx/EhQodDF20oHLMixoRaWcrejinIf) — 完整架构图、PUF 接入方案、SDK 设计

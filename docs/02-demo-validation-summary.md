# ldap-demo 验证总结

## 测试环境

Docker Compose，三容器：OpenLDAP、ldap-init（一次性初始化）、nas（Go authd + Samba + NFS）。

## 验证结果

### 用户注册与登录

| 测试项 | 结果 |
|--------|------|
| 注册写入 LDAP | ✓ uidNumber 自动递增（alice=1001, bob=1002） |
| 注册创建 Linux 用户 | ✓ `id alice` 返回正确 UID/GID |
| 注册创建数据目录 | ✓ `/data/alice` 权限 700 |
| 登录 LDAP Bind 验证 | ✓ 密码正确返回 JWT，错误返回 401 |
| JWT Token 验证 | ✓ `/validate-token` 返回 `{"valid":true,"username":"alice"}` |

### SMB 文件访问

| 测试项 | 结果 |
|--------|------|
| SMB 登录 | ✓ ldapsam 方案，注册时自动写入 sambaSamAccount，无需手动 smbpasswd |
| alice 写自己目录 | ✓ `put` 文件成功 |
| bob 写 alice 目录 | ✓ `NT_STATUS_ACCESS_DENIED`（正确拒绝） |
| 注册自动同步 Samba | ✓ ldapsam 验证通过 |

**排查记录：NT_STATUS_INVALID_SID**

重建容器后 SMB 登录报 `NT_STATUS_INVALID_SID`。根因：Samba 首次启动随机生成域 SID，与 authd 注册时写入 LDAP 的 `sambaSID` 前缀不一致。

修复：在 `start.sh` 中加入 `net setlocalsid "${DOMAIN_SID}"`，`DOMAIN_SID` 通过 docker-compose 环境变量统一配置。注意 `smb.conf` 中不存在 `ldap domain sid` 参数，该写法无效。

### 权限共享

| 测试项 | 结果 |
|--------|------|
| alice 授权 bob 只读 | ✓ API 返回 `{"ok":true}` |
| bob 写 alice 目录 | ✓ 正确拒绝 |
| bob 读 alice 目录 | ✓ 授权时同时执行 `chmod g+x`，SMB 预检通过 |

`/share/permission` 接口 action 字段验证结果：

| action | ACL | 验证结果 |
|--------|-----|----------|
| `readonly` | r-x | ✓ bob 可读不可写 |
| `readwrite` | rwx | ✓ bob 可读可写 |
| `remove` | 移除 | ✓ bob 完全拒绝访问 |

`remove` 同时清除递归 ACL 和默认 ACL（`setfacl -R -x user:bob` + `default:user:bob`）。

### WebDAV 统一认证

| 测试项 | 结果 |
|--------|------|
| `/validate-token` 接口 | ✓ 可作为 nginx auth_request 后端 |

### NFS

| 测试项 | 结果 |
|--------|------|
| UID 映射机制 | ✓ 验证通过 |
| 用户级认证 | → 需 Kerberos，demo 阶段 IP 白名单替代 |

## 遗留问题

1. **ACL 只读共享对 SMB 的注意事项**：授权时需同时执行 `chmod g+x`，见文档03

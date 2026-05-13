# NAS 连接鉴权与权限管理方案

## 一、整体架构

### 第一阶段：连接/登录认证

```
客户端
  ├─ HTTP/WebDAV ──→ authd ──────────────→ LDAP Bind 验证 ──→ 签发 JWT
  ├─ SMB ──────────→ Samba (ldapsam) ──→ LDAP Bind 验证 ──→ 建立会话
  └─ NFS ──────────→ Kerberos ──────────→ KDC 验证 ticket ──→ 建立会话
                ↑
           LDAP（唯一身份数据源）
```

### 第二阶段：每次文件操作时的权限控制

```
客户端发起文件操作（读/写/列目录）
  │
  ├─ WebDAV ──→ authd 收到请求
  │              ├─ 验证 JWT（确认身份）
  │              └─ 读 POSIX ACL（确认权限）──→ 有权限：执行 / 无权限：HTTP 403
  │
  ├─ SMB ─────→ Samba 以该用户 UID 发起 syscall
  │              └─ 内核检查 POSIX ACL ──→ 有权限：执行 / 无权限：ACCESS_DENIED
  │
  └─ NFS ─────→ NFS 服务端以客户端 UID 发起 syscall
                 └─ 内核检查 POSIX ACL ──→ 有权限：执行 / 无权限：EACCES
```

### 权限写入（管理后台）

```
管理员操作 → authd /admin/permission → setfacl → 文件系统 POSIX ACL
                                                        ↓ 自动生效
                                          SMB / NFS / WebDAV 第二阶段
```

---

## 二、用户身份管理

**唯一数据源：LDAP**

| 字段 | 说明 |
|------|------|
| uid | 用户名，全局唯一 |
| uidNumber | Linux UID，从 1001 递增，不回收 |
| gidNumber | 固定 1000（nas-users 组） |
| userPassword | SSHA hash（LDAP Bind 验证用） |
| sambaNTPassword | NT hash（Samba ldapsam 验证用） |
| sambaSID | Samba 安全标识符 |

**注册时一次性写入 LDAP，其他系统无需单独维护：**

1. LDAP 条目（含 posixAccount + sambaSamAccount）
2. Linux 系统用户（UID 与 LDAP 一致，供文件系统权限检查）
3. `/data/<username>` 目录，初始权限 700

---

## 三、各协议鉴权方式

### HTTP / WebDAV

- `POST /login` 用 LDAP Bind 验证密码，签发 JWT（24h）
- 每次请求验证 JWT 身份，再读 POSIX ACL 决定是否放行
- 权限检查精确到每次文件操作

### SMB

- Samba 使用 `ldapsam` 后端，直接查 LDAP 验证密码
- 无需维护独立的 tdbsam，密码变更 LDAP 改一处即生效
- 文件操作权限由内核检查 POSIX ACL

### NFS（生产方案）

- NFSv4 + Kerberos，KDC 可对接 LDAP
- 文件操作权限同样由内核检查 POSIX ACL
- Demo 阶段用 IP 白名单 + root_squash 替代

---

## 四、权限模型

**POSIX ACL 是唯一权限真相**，管理后台写入，三个协议直接读取。

| 级别 | POSIX ACL | SMB 表现 | WebDAV 表现 | NFS 表现 |
|------|-----------|---------|------------|---------|
| 无权限 | `---` | ACCESS_DENIED | HTTP 403 | EACCES |
| 只读 | `r-x` | 可读/列目录 | GET 允许，PUT/DELETE 拒绝 | 只读 |
| 读写 | `rwx` | 读写均可 | 所有方法允许 | 读写 |

**授权操作（管理后台触发）：**

```bash
# 只读授权
chmod g+x /data/alice                              # 让 Samba 预检通过
setfacl -R -m user:bob:r-x /data/alice/photos
setfacl -R -d -m user:bob:r-x /data/alice/photos  # 新建文件继承

# 撤销授权
setfacl -R -x user:bob /data/alice/photos
```

---

## 五、权限管理后台接口

| 接口 | 说明 |
|------|------|
| `POST /share/permission` | 设置权限，`action` 字段控制操作类型 |

**请求体：**

| action | 效果 | 等价 setfacl |
|--------|------|-------------|
| `readonly`（或 `readonly:true`） | 只读 `r-x` | `-m user:bob:r-x` |
| `readwrite`（或 `readonly:false`） | 读写 `rwx` | `-m user:bob:rwx` |
| `remove` | 完全移除 | `-x user:bob`（递归+默认ACL） |

**示例：**

```bash
# 只读授权
curl -X POST http://localhost:8080/share/permission \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"/data/alice","target_user":"bob","action":"readonly"}'

# 读写授权
curl -X POST http://localhost:8080/share/permission \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"/data/alice","target_user":"bob","action":"readwrite"}'

# 撤销授权
curl -X POST http://localhost:8080/share/permission \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"/data/alice","target_user":"bob","action":"remove"}'
```

---

## 六、数据结构

### LDAP 用户条目

注册时一次性写入，包含 `posixAccount + sambaSamAccount` 两个 objectClass：

| 字段 | 值 | 用途 |
|------|----|----|
| `uidNumber` | 1001, 1002, ... | Linux UID，文件系统权限主体，不回收 |
| `gidNumber` | 1000 | 固定，nas-users 组 |
| `userPassword` | `{SSHA}...` | HTTP/WebDAV 登录验证（LDAP Bind） |
| `sambaNTPassword` | MD4(UTF-16LE(pwd)) | SMB ldapsam 验证 |
| `sambaSID` | `S-1-5-21-...-<uid*2+1000>` | Samba 身份标识 |

### 文件系统权限（POSIX ACL）

POSIX ACL 是唯一权限真相，三协议共用：

```
/data/<owner>/              chmod 700 + ACL
  user:<owner>:rwx          所有者完全控制
  user:<other>:r-x          只读授权（readonly=true）
  user:<other>:rwx          读写授权（readonly=false）
  default:user:<other>:r-x  新建文件自动继承
```

授权时必须同时执行 `chmod g+x`，否则 Samba 在 POSIX ACL 之前就因 `other=---` 拒绝请求。

### JWT Payload

```json
{ "sub": "alice", "iat": <unix>, "exp": <iat+86400> }
```

有效期 24 小时，用 `HS256` 签名。

---

## 七、JWT 详解

JWT 结构：三段 Base64URL 编码，`.` 分隔：Header / Payload / Signature

- Header：`{ "alg": "HS256", "typ": "JWT" }`
- Payload：`{ "sub": "alice", "iat": 1715000000, "exp": 1715086400 }`
- Signature：`HMAC-SHA256(base64url(header) + "." + base64url(payload), secret)`

验证流程：服务端重新计算签名，对比 token 中签名，签名一致且未过期则取出 sub=用户名放行，否则 401。服务端不存储 token，无状态。

**国密算法替换方案：**

| 标准算法 | 国密替代 | 说明 |
|----------|----------|------|
| HS256（对称） | HMAC-SM3 | 用 SM3 替换 SHA-256，密钥仍为共享密钥 |
| RS256（非对称） | SM2withSM3 | 用 SM2 私钥签名，公钥验证，适合多服务场景 |

**为什么不需要应用层加密密码：** HTTPS 已足够。TLS 握手后整个请求体均已加密传输，应用层再加密属于重复工作。正确做法：确保全站强制 HTTPS，禁止 HTTP 降级（HSTS）。

---

## 八、局限性

| 问题 | 说明 |
|------|------|
| SMB 与 POSIX ACL 映射 | Samba 某些配置下优先检查 Unix 权限位，授权时需配合 `chmod g+x` |
| NFS 无用户级认证 | 依赖 UID 一致性，生产需 Kerberos |
| WebDAV 需主动读 ACL | 每次请求调 `getfacl` 或缓存结果 |

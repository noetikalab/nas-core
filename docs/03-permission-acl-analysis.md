# 权限管理缺陷分析：文件协议与 POSIX ACL 的不一致性

## 一、根本矛盾

POSIX ACL 是 Linux 内核的权限模型。SMB 和 NFS 在文件操作时都经过内核，理论上都能读 POSIX ACL。但 SMB（Samba）在内核检查之前，有自己的一层权限预检，导致行为不一致。

---

## 二、SMB 的问题

**现象**：`setfacl -R -m user:bob:r-x /data/alice` 执行成功，但 bob 通过 SMB 访问仍然 ACCESS_DENIED。

**原因**：Samba 在把请求交给内核之前，会先检查目录对 `other` 的 Unix 权限位。alice 目录是 `chmod 700`，other=`---`，Samba 看到这个就直接拒绝，POSIX ACL 根本没有被检查到。

**解决方案**：授权时同时修改目录权限，让 Samba 的预检通过，再由 POSIX ACL 做精细控制：

```bash
# 授权 bob 只读时，同时执行：
chmod g+x /data/alice          # 让 nas-users 组成员可以进入目录
setfacl -m user:bob:r-x /data/alice
setfacl -d -m user:bob:r-x /data/alice
```

这样 Samba 预检看到 group 有 `x`，放行后内核再用 POSIX ACL 决定 bob 具体能做什么。

---

## 三、NFS 的问题

NFS 文件操作完全走内核，POSIX ACL 直接生效，没有额外的预检层。行为与预期一致。

唯一的问题是认证层：NFS 信任客户端上报的 UID，没有密码验证。生产环境需要 Kerberos 解决。

---

## 四、WebDAV 的问题

WebDAV 由 authd 处理，需要主动读取 POSIX ACL 来决定是否放行，不像 SMB/NFS 那样由内核自动检查。

实现方式：每次文件请求时以目标用户的 UID 调用 `syscall.Access` 检查权限。

---

## 五、协议权限一致性对比

| | SMB | NFS | WebDAV |
|--|-----|-----|--------|
| 认证层 | LDAP（ldapsam） | Kerberos/UID | JWT |
| 权限检查层 | Samba预检 + 内核POSIX ACL | 内核POSIX ACL | authd主动检查 |
| POSIX ACL 生效 | 需绕过预检 | 直接生效 | 需主动读取 |
| 权限一致性 | 基本一致（需配合chmod） | 完全一致 | 完全一致 |

---

## 六、结论

**权限管理后台的正确实现**：每次设置权限时，同时执行两步：

```bash
chmod g+x <目录>                        # 让 Samba 预检通过
setfacl -m user:<user>:<perm> <目录>    # 精细权限控制
```

这样三个协议的行为趋于一致，POSIX ACL 是唯一的权限数据源。

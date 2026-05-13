#!/bin/bash
# 容器启动脚本，按顺序启动所有服务

# set -e：任何命令失败就立即退出，防止带着错误继续运行
set -e

# 创建 nas-users 组（GID=1000）
# 注册用户时 useradd -g 1000 依赖这个组存在
# 2>/dev/null || true：组已存在时 groupadd 会报错，忽略这个错误继续执行
groupadd -g 1000 nas-users 2>/dev/null || true

# 告诉 Samba LDAP 管理员密码是什么
# Samba 需要用这个密码去 LDAP 查用户信息，必须和 smb.conf 里的 ldap admin dn 对应
(echo "admin123"; echo "admin123") | smbpasswd -w admin123 2>/dev/null || true

# 设置域 SID，必须与 authd 写入 LDAP 的 sambaSID 前缀一致
net setlocalsid "${DOMAIN_SID}" 2>/dev/null || true

# 配置并启动 NFS 服务
# /etc/exports 定义哪些目录对外共享，以及权限
# * 表示允许所有 IP 挂载（demo 用，生产环境应限制 IP 段）
# rw：允许读写
# sync：写操作同步到磁盘后才返回（数据安全）
# no_subtree_check：不检查子目录，提高性能
# no_root_squash：客户端 root 用户不被压缩成匿名用户（demo 用）
echo "/data *(rw,sync,no_subtree_check,no_root_squash)" > /etc/exports

rpcbind  || true   # RPC 端口映射服务，NFS 依赖它
rpc.nfsd || true   # NFS 服务主进程
rpc.mountd || true # 处理客户端 mount 请求
exportfs -ra || true # 重新加载 /etc/exports 配置

# 后台启动 Samba
# smbd：处理文件共享和用户认证
# nmbd：处理 NetBIOS 名称解析（Windows 网络发现）
# --foreground --no-process-group：不 daemonize，方便 Docker 管理进程
smbd --foreground --no-process-group &
nmbd --foreground --no-process-group &

# 前台启动 Go HTTP 服务，作为容器主进程
# exec 替换当前 shell 进程，Docker 的信号（如 Ctrl+C）会直接发给 authd
exec /usr/local/bin/authd

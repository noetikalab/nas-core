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

# 后台启动 Nginx（WebDAV，端口 8081）
nginx &

# 后台启动 Samba
# smbd：处理文件共享和用户认证
# nmbd：处理 NetBIOS 名称解析（Windows 网络发现）
# --foreground --no-process-group：不 daemonize，方便 Docker 管理进程
smbd --foreground --no-process-group &
nmbd --foreground --no-process-group &

# ====================================================================
# WiFi Direct P2P — NAS 作为 Group Owner
# 手机通过 WiFi Direct 直连 NAS，无需路由器
# 接口名 p2p-wlp3s0-*，GO IP 固定 192.168.49.1
# DHCP 池 192.168.49.100~200，租约 12 小时
#
# 环境自适应：
#   - Server（无 NetworkManager）：容器内启动 wpa_supplicant + 创建 P2P
#   - 桌面版（有 NetworkManager/D-Bus）：P2P 不可用，跳过
# ====================================================================
if iw dev | grep -q wlp; then
    # 推导设备 ID，逻辑与 authd/mdns/server.go 的 deviceID() 一致
    P2P_DID="${DEVICE_ID:-$(hostname)}"
    P2P_NAME="NAS-${P2P_DID}"
    # wpa_supplicant device_name 限制 32 字符，超长会导致 P2P 创建失败
    P2P_NAME="${P2P_NAME:0:32}"

    if ps aux | grep -q "[w]pa_supplicant.*-u"; then
        # ── 桌面版 Ubuntu（wpa_supplicant D-Bus 模式）──
        # NetworkManager 通过 D-Bus 接管 WiFi，此模式下 P2P 命令不可用，
        # 后续 NAS 硬件（Ubuntu Server）无 NetworkManager，P2P 自动可用
        echo "[P2P] 桌面环境 (D-Bus 模式) — P2P 不可用，请使用 mDNS 局域网发现"
    else
        # ── Server 版 Ubuntu 或 wpa_supplicant 未运行 ──
        # 容器内启动 wpa_supplicant，直接控制 WiFi 硬件
        echo "[P2P] Server 环境，容器内初始化 P2P ($P2P_NAME)"

        # 启动 wpa_supplicant 守护进程（如果尚未运行）
        wpa_supplicant -B -i wlp3s0 -D nl80211 2>/dev/null || true
        sleep 1

        # 设置 WiFi Direct 设备名，APP 通过 deviceName.contains("nas") 识别
        wpa_cli -i wlp3s0 set device_name "$P2P_NAME" 2>/dev/null || true

        # 创建 P2P Group，NAS 成为 Group Owner
        wpa_cli -i wlp3s0 p2p_group_add persistent 2>/dev/null || true
        sleep 3

        # 查找系统自动创建的 P2P 虚拟接口（名如 p2p-wlp3s0-0）
        P2P_IFACE=$(iw dev | grep -o 'p2p-wlp3s0[^ ]*' | head -1)

        if [ -n "$P2P_IFACE" ]; then
            # 分配固定 IP 作为 Group Owner 地址
            ip addr add 192.168.49.1/24 dev "$P2P_IFACE" 2>/dev/null || true
            ip link set "$P2P_IFACE" up

            # DHCP 服务：给连接的手机自动分配 IP
            dnsmasq \
                --interface="$P2P_IFACE" \
                --dhcp-range=192.168.49.100,192.168.49.200,255.255.255.0,12h \
                --no-daemon \
                --log-facility=- \
                2>/dev/null &

            echo "[P2P] Ready — $P2P_NAME @ $P2P_IFACE → 192.168.49.1"
        else
            echo "[P2P] WARN: P2P 虚拟接口未创建"
        fi
    fi
else
    echo "[P2P] 无 WiFi 硬件，跳过"
fi

# 前台启动 Go HTTP 服务，作为容器主进程
# exec 替换当前 shell 进程，Docker 的信号（如 Ctrl+C）会直接发给 authd
exec /usr/local/bin/authd

# avahi-browse mDNS 调试

## 基本用法

```bash
# 浏览所有 mDNS 服务
avahi-browse -a -t

# 只浏览特定类型
avahi-browse _nas._tcp -t

# 解析服务（显示 IP 和端口）
avahi-browse -r _nas._tcp -t

# 详细输出
avahi-browse -v -r _nas._tcp -t

# 指定网络接口
avahi-browse -r _nas._tcp -t -i wlp3s0
```

## 输出解读

```
= wlp3s0 IPv4 NAS-zhangli-ASUS-TUF                    _nas._tcp           local
   hostname = [zhangli-ASUS-TUF.local]
   address = [10.20.132.121]
   port = [8080]
   txt = ["host=zhangli-ASUS-TUF" "version=1.0"]
```

- `=` 表示服务在线
- `wlp3s0` 是网络接口（WiFi 物理网卡）
- `_nas._tcp` 是服务类型
- `address` 是设备 IP
- `port` 是服务端口

**关键检查**：IP 是否正确（非 Docker bridge IP `172.x.x.x`），端口是否为 `8080`。

## 常见问题

### 看不到服务

1. 确认 mDNS 服务端正在运行
2. NAS 是否用了 Docker `network_mode: host`（非 host 模式多播出不去）
3. 防火墙是否阻挡了 UDP 5353

### IP 是 Docker 内部地址

Go zeroconf 的 `pickIP()` 需要过滤 `172.17.0.0/16`、`172.18.0.0/16` 等 Docker bridge 子网。检查 `authd/mdns/server.go` 中的 IP 过滤逻辑。

### 与 Android NsdManager 的差异

`avahi-browse` 比 Android NsdManager 更宽松，avahi 能看到不意味着 Android 能正常 resolve。如果 avahi 能看到但 Android 不能 resolve，需要抓包对比 mDNS 响应结构。

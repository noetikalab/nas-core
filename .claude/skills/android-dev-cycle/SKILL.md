---
name: android-dev-cycle
description: RN + Android 原生开发工作流：每次修改 Android 原生代码（Kotlin/Java/XML）后，自动执行构建→开发服务器→日志查看循环。构建失败时分析错误并等待用户确认再修复。涵盖 adb、avahi-browse、tcpdump 等调试工具，以及 mDNS、权限、网络等常见坑点。当用户修改原生代码、说"编译"、"构建"、"build"、"查看日志"、或遇到构建失败时触发。
---

# Android + RN 开发工作流

## 核心流程

每次修改 Android 原生代码（`android/app/src/main/java/` 下的 `.kt`/`.java`、`AndroidManifest.xml`、`build.gradle`）后：

```
┌─────────────────────────────────────────────────────────┐
│  1. pnpm android   编译并安装到设备                        │
│     ├─ 成功 → 步骤 2                                      │
│     └─ 失败 → 提取错误 → 分析 → 告知用户 → 等待确认 → 修复  │
│                                                          │
│  2. pnpm dev        启动 Metro 开发服务器（adb reverse）     │
│                                                          │
│  3. adb logcat -s TAG  查看日志（TAG 替换为实际标签）        │
└─────────────────────────────────────────────────────────┘
```

## 规则

### 构建失败处理

当 `pnpm android` 编译失败时，**不要直接修改代码**：

1. 从输出中提取 `FAILED`、`error:` 开头的行
2. 定位文件和行号
3. 用一两句话解释根因
4. 提出修复方案，**等待用户确认**后再改
5. 修复后回到步骤 1 重新构建

常见的 Kotlin 编译错误：
- `Unresolved reference` — 变量名/方法名写错，或缺少 import
- `Type mismatch` — 类型不匹配
- `Unresolved reference 'reactContext'` — 在 `ReactContextBaseJavaModule` 中应该用 `reactApplicationContext`

### 用户操作节点

以下场景需要**明确引导用户**执行操作：

| 场景 | 引导语 |
|------|--------|
| 构建安装完成 | "已安装到设备，请打开 APP 并进入 XXX 页面触发功能" |
| 需要查看日志 | "请操作 APP 触发对应功能，我会在这里监控日志" |
| 需要抓 mDNS 包 | "请在 NAS 上执行 `tcpdump -i wlp3s0 port 5353 -n`，然后重新触发 APP 扫描" |
| NAS 是否运行 | "请在 NAS 上执行 `sudo docker compose ps` 确认容器运行状态" |
| 验证 NAS 可达 | "请用手机浏览器打开 `http://<IP>:8080/ping` 确认 HTTP 可达" |

### 日志监控

监控日志使用 `adb logcat`，启动方式：

```bash
adb logcat -c && adb logcat -s MdnsModule   # 清空并只显示 MdnsModule 标签
adb logcat -c && adb logcat -s MdnsModule WifiP2pModule NfcModule  # 多标签
adb logcat -c && adb logcat | grep -E "MdnsModule|ReactNative"  # 多标签 + grep
```

关键：`-c` 先清空缓冲区避免历史日志干扰。如果用户要在手机上操作触发日志，用 Monitor 工具持续采集。

日志中关注 `Log.e()` 输出的异常信息，以及关键状态值（如 `host=null port=0` 说明 resolve 未完成）。

---

## 工具参考

### adb

| 命令 | 用途 |
|------|------|
| `adb devices` | 列出已连接设备 |
| `adb logcat -c` | 清空日志缓冲区 |
| `adb logcat -s <TAG>` | 按标签过滤日志 |
| `adb logcat -d` | 输出当前缓冲并退出（不持续监控） |
| `adb reverse tcp:8081 tcp:9173` | USB 端口转发（Metro bundler） |
| `adb reverse --list` | 查看当前转发规则 |
| `adb install <apk>` | 直接安装 APK |

更多细节参考 [references/adb.md](references/adb.md)

### avahi-browse（mDNS 调试，NAS 侧）

| 命令 | 用途 |
|------|------|
| `avahi-browse -a -t` | 列出所有 mDNS 服务（仅名称） |
| `avahi-browse -r _nas._tcp -t` | 解析 `_nas._tcp` 服务，显示完整记录（IP/端口/TXT） |
| `avahi-browse -v -r _nas._tcp -t` | 更详细的输出 |

更多细节参考 [references/avahi.md](references/avahi.md)

### tcpdump（mDNS 抓包，NAS 侧）

```bash
sudo tcpdump -i wlp3s0 port 5353 -n   # 抓 mDNS 包
sudo tcpdump -i any port 5353 -n -v    # 所有接口，详细输出
```

分析时关注：PTR 响应中是否附带了 SRV/TXT/A 记录（additional section），以及 Android 发出 SRV 查询后 NAS 是否有回应。

---

## 常见坑点

参考 [references/common-issues.md](references/common-issues.md)，目前收录：

1. **mDNS serviceType 末尾带 `.`**：`NsdManager.onServiceFound` 返回的 `serviceType` 是 DNS FQDN 格式（如 `_nas._tcp.`），比较时必须 `removeSuffix(".")`
2. **mDNS 权限**：必须声明 `CHANGE_WIFI_MULTICAST_STATE`、`ACCESS_WIFI_STATE`、`ACCESS_NETWORK_STATE`，缺一个都会导致 discovery 返回空
3. **Docker network_mode**：NAS 容器用 `network_mode: host`，否则 mDNS UDP 多播包被 bridge 隔离
4. **Docker bridge IP**：Go zeroconf 的 `pickIP()` 必须过滤 `172.17-31.x.x` 网段，否则广播 Docker 内部 IP
5. **Metro 端口冲突**：默认 8081 可能与 NAS WebDAV 冲突，改 `devServerPort: 9173`
6. **USB 安装被拒**：手机开发者选项 → 打开"USB 安装"
7. **AsyncStorage v3 不可用**：v3 依赖国内镜像没有的 Maven 包，必须用 v2

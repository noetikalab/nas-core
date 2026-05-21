# 常见坑点汇总

## 1. mDNS serviceType 末尾带 `.`

**现象**：`onServiceFound` 触发，但永远不会调用 `resolveService`，5s 超时后返回 0 设备。

**根因**：Android `NsdManager` 返回的 `serviceType` 是 DNS FQDN 格式，末尾带 `.`（如 `_nas._tcp.`）。代码中 `SERVICE_TYPE = "_nas._tcp"` 没有 `.`，字符串比较失败。

**修复**：
```kotlin
if (service.serviceType.removeSuffix(".") != SERVICE_TYPE) return
```

**见识**：`onServiceFound` 触发了不代表代码走到了 `resolveService`，加日志验证。

## 2. mDNS 必须声明 3 个权限

**现象**：`discover()` 总是返回空数组，logcat 无任何日志输出。

**根因**：缺少多播权限，`NsdManager` 静默失败。

**修复**：`AndroidManifest.xml` 中声明：
```xml
<uses-permission android:name="android.permission.CHANGE_WIFI_MULTICAST_STATE" />
<uses-permission android:name="android.permission.ACCESS_WIFI_STATE" />
<uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />
```

`INTERNET` 权限也必须有。四个权限缺一不可。

## 3. Docker network_mode 阻断 mDNS

**现象**：NAS 物理机 `avahi-browse` 能看到服务，但其他设备收不到广播。

**根因**：Docker bridge 网络不转发 UDP 多播包到物理网卡。

**修复**：`docker-compose.yml` 中 nas 容器用 `network_mode: host`，并移除 `ports:` 段。

> ⚠️ `network_mode: host` 仅 Linux 有效，Docker Desktop Mac/Windows 不支持。

## 4. Docker bridge IP 被广播

**现象**：`avahi-browse` 看到 IP 是 `172.18.x.x`（Docker 内部地址），外部设备无法连接。

**根因**：Go zeroconf 的 `RegisterProxy` 默认选取第一个 IP，而 Docker 网桥 `172.17.0.0/16`、`172.18.0.0/16` 等排在最前。

**修复**：`authd/mdns/server.go` 中 `pickIP()` 跳过 `172.17.0.0/16 ~ 172.31.0.0/16` 范围的 IP，只保留物理网卡 IP。

## 5. ReactApplicationContext vs reactContext

**现象**：编译错误 `Unresolved reference 'reactContext'`。

**根因**：在 `ReactContextBaseJavaModule` 子类中，构造参数名是 `reactContext` 但成员变量是 `reactApplicationContext`。

**修复**：
```kotlin
class MdnsModule(reactContext: ReactApplicationContext) : ReactContextBaseJavaModule(reactContext) {
    // 使用 reactApplicationContext，不是 reactContext
    val executor = reactApplicationContext.mainExecutor
}
```

## 6. Metro 端口 8081 冲突

**现象**：Metro bundler 启动失败或报端口占用。

**根因**：NAS WebDAV 用 8081，Metro 默认也是 8081。

**修复**：`package.json` 的 `dev` 命令中配置 `devServerPort: 9173`，`adb reverse tcp:8081 tcp:9173` 映射。

## 7. Kotlin 字符串插值下划线

**现象**：编译错误 `Unresolved reference '$_SERVICE_TYPE'`。

**根因**：Kotlin 将 `$_SERVICE_TYPE` 解析为变量 `$_SERVICE_TYPE` 而不是 `$` + `SERVICE_TYPE`。

**修复**：用 `${SERVICE_TYPE}` 而非 `$SERVICE_TYPE`，或直接用字符串拼接。

## 8. USB 安装被拒（INSTALL_FAILED_USER_RESTRICTED）

**修复**：手机 设置 → 开发者选项 → 打开"USB 安装"。

## 9. AsyncStorage v3 构建失败

**根因**：v3 依赖的 `org.asyncstorage.shared_storage:storage-android` 在国内 Maven 镜像中不存在。

**修复**：用 `@react-native-async-storage/async-storage` v2 版本，不要升级 v3。

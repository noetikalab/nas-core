# adb 常用命令参考

## 设备连接

```bash
adb devices                          # 列出已连接设备
adb -s <serial> <command>            # 指定设备执行命令
adb connect <ip>:5555                # WiFi 连接（需先 USB 开启 tcpip 5555）
adb disconnect                       # 断开 WiFi 连接
adb kill-server                      # 重启 adb 服务端
```

## 日志

```bash
adb logcat -c                        # 清空日志缓冲
adb logcat -s <TAG>                  # 只显示指定标签
adb logcat -s <TAG1> <TAG2>          # 多标签
adb logcat -d                        # dump 当前缓冲并退出
adb logcat -v time                   # 带时间戳格式
adb logcat -v threadtime             # 带线程+时间戳
adb logcat -e "<regex>"              # grep 过滤
adb logcat --pid=<pid>               # 按进程 PID 过滤
```

## 安装

```bash
adb install <app.apk>                # 安装 APK
adb install -r <app.apk>             # 覆盖安装（保留数据）
adb install -t <app.apk>             # 允许安装 test-only APK
adb uninstall <package.name>         # 卸载
```

## 端口转发

```bash
adb reverse tcp:8081 tcp:9173        # 手机 8081 → 电脑 9173（Metro bundler）
adb reverse --list                   # 查看当前转发
adb reverse --remove tcp:8081        # 删除某条转发
```

## Shell

```bash
adb shell                            # 进入设备 shell
adb shell ping <ip>                  # 测试网络连通
adb shell ip addr show wlan0         # 查看 WiFi IP
adb shell dumpsys wifi               # WiFi 状态
adb shell cmd wifi list-networks     # WiFi 网络列表
adb shell pm list packages           # 列出已安装包
```

## 设备信息

```bash
adb shell getprop ro.build.version.sdk   # SDK 版本
adb shell getprop ro.product.model       # 设备型号
adb shell getprop ro.product.manufacturer # 制造商
```

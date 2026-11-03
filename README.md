# eCaptureLens

Android eCapture 流量分析器。可移植 Go 核心、完整 `ecapturelens` CLI、Flutter Web/Android 界面共用同一个业务入口。PC 用浏览器访问 Go 服务，手机可独立运行，也可连接电脑上的服务。

## 构建与启动

需要 Go 1.26；图形界面构建需要 Flutter stable。PC 不需要 Xcode 或桌面原生工具链，成品只需运行 Go 可执行文件并打开浏览器。Android 原生构建需要 Android SDK/NDK。

```sh
make web
./build/desktop/ecapturelens web
```

打开命令输出的 `url`。PC 成品位于 `build/desktop/`，包含可执行文件和 `web/` 静态资源，分发时一起复制。Windows 可执行文件名为 `ecapturelens.exe`；同一份 Web 资源可以搭配 Windows、macOS 或 Linux 的 Go 可执行文件。Web 默认监听 `127.0.0.1:8787`，可用 `--listen` 修改，`--web-dir` 指定其他静态资源目录。

`web` 自动生成 API 令牌，放在启动链接的 `#token=...` 中；页面读取后清除地址栏片段，只在当前标签页的会话存储中保留。API 仍通过 Bearer 认证；界面“API → Copy CLI connection”可以复制 Agent 所需的地址和令牌。关闭标签页结束会话，Go 进程负责持续抓包和历史保存；终端 Ctrl+C 才停止核心。

只需 CLI/API 时：

```sh
make build
./bin/ecapturelens --help
./bin/ecapturelens --data-dir .data serve
```

手机端 CLI 可通过 `bash scripts/build-cli.sh android` 构建到 `bin/android-arm64/ecapturelens`，通过 ADB 推送执行；用 `--data-dir` 指定可写目录。它与 APK 内置核心及电脑 CLI 使用同一业务实现。

服务默认监听 `127.0.0.1:8787`。Agent 可以用 CLI 分步控制正在运行的服务：

```sh
./bin/ecapturelens --server http://127.0.0.1:8787 api describe
./bin/ecapturelens --server http://127.0.0.1:8787 requests list --query example
./bin/ecapturelens --server http://127.0.0.1:8787 body view REQUEST_ID --side response --format json
./bin/ecapturelens --server http://127.0.0.1:8787 history export --format har --output traffic.har
```

同一数据目录只能由一个核心进程持有；服务运行时使用 `--server` 访问。省略 `--server` 的 CLI 命令直接调用核心。本地 `capture start` 在前台持续运行，Ctrl+C 停止；服务模式的启停命令立即返回，可用 `capture status` 检查结果。

```sh
# 构建内嵌 Go 核心和 Android arm64 APK
bash scripts/build-ui.sh android

# PC：Flutter Web + Go
bash scripts/build-ui.sh web
```

可通过 `FLUTTER` 指定 Flutter 可执行文件；本次开发 SDK 位于 `$HOME/.cache/ecapturelens/flutter/bin/flutter`。Android 默认 NDK 为 `28.2.13676358`，支持 `ANDROID_NDK_HOME` / `ANDROID_SDK_ROOT`。详细构建步骤见 [ui/README.md](ui/README.md)。APK 内置核心，独立使用不要求手机安装 Flutter、Go 或 CLI。

## 接入 eCapture

推荐 eCapture v2.9.1 或更高版本，使用 `tls --text-source pcapng` 复用它的 TLS/HTTP 重建能力。Android 需要 root、可用的 eBPF 内核及正确的 libssl 路径/版本。界面“设备”可以设置任意启动/停止命令；命令在运行核心的机器执行。手机配置 `su -c '...'`，电脑配置 `adb ...`。

以下是手机上手动启动的示例，网卡、libssl、过滤条件需要匹配实际设备：

```sh
su -c '/data/local/tmp/ecapture tls --text-source pcapng -i wlan0 \
  --libssl=/apex/com.android.conscrypt/lib64/libssl.so \
  --ssl_version="boringssl 1.1.1" \
  --ecaptureq=ws://127.0.0.1:28257/ \
  --pcapfile=/data/local/tmp/traffic.pcapng tcp port 443'
```

手机界面新增设备，endpoint 填 `ws://127.0.0.1:28257/`，即可连接；也可把上述命令设为启动命令，并填写对应的停止命令。为手机应用授予 root 权限后，按钮便可控制本机抓包。eCapture 二进制由使用者安装并配置路径。

电脑分析 USB 手机流量时先启动手机 eCapture，再转发端口：

```sh
adb -s SERIAL forward tcp:28257 tcp:28257
./bin/ecapturelens --server http://127.0.0.1:8787 devices put pixel \
  --name 'Pixel' --endpoint ws://127.0.0.1:28257/
./bin/ecapturelens --server http://127.0.0.1:8787 capture start pixel
./bin/ecapturelens --server http://127.0.0.1:8787 capture status
./bin/ecapturelens --server http://127.0.0.1:8787 capture stop pixel
```

启动命令可以是持续前台进程，也可以是一次性启动器。前台进程的输出会进入抓包日志；进程退出、握手失败、断线、解析或持久化失败会显示为 error。只在刚启动的进程尚未监听时，对连接拒绝等待至多 10 秒；不会在断线后静默重连。停止会执行该次启动时绑定的停止命令，并终止自有进程组。

## 分析能力

- 请求/响应、HTTP/1 FIFO 配对、HTTP/2 stream ID 多路复用、多设备隔离、会话时间线。
- JSON 格式化、文本、Hex、复制、JSON/HAR 导出与导入；保留全局最新 1000 条交易，含待响应记录，重启恢复。
- gzip、deflate、Brotli 与组合 Content-Encoding；Protobuf 无模式字段查看或 FileDescriptorSet 命名解码；gRPC 帧及 gzip 压缩消息。
- 业务解码器：注册命令，Body 字节从 stdin 输入，UTF-8 结果从 stdout 返回，失败保留错误。
- CLI、HTTP 自动化接口、PC Web 和 Android 本地/远程连接共用所有业务操作。

```sh
./bin/ecapturelens --server http://127.0.0.1:8787 decoders put my-protocol --command 'python3 /path/decode.py'
./bin/ecapturelens --server http://127.0.0.1:8787 body view REQUEST_ID --format my-protocol

protoc --include_imports --descriptor_set_out=api.pb api.proto
./bin/ecapturelens --server http://127.0.0.1:8787 body view REQUEST_ID \
  --format grpc --descriptor api.pb --message-type example.Response
```

完整命令、参数和 Agent 用法见 [CLI 手册](docs/cli.md)，共享接口见 [接口契约](docs/contract.md)。界面还提供通用 API 操作入口，便于直接调用同一业务方法。

## HTTP 自动化

`POST /api` 接受 `{ "method": "requests.list", "params": {} }`，返回 `{ "result": ... }` 或带非 2xx 状态的 `{ "error": "..." }`。`api.describe` 列出所有操作。监听局域网必须配置 token：

```sh
./bin/ecapturelens --data-dir .data --token YOUR_TOKEN serve --listen 0.0.0.0:8787
./bin/ecapturelens --server http://PC_IP:8787 --token YOUR_TOKEN requests list
```

Android 远程设置填写相同服务地址/token，也可以连接 PC 的 `web` 服务。PC 浏览器使用所在服务的同源 API，不开放跨域 API；配置的抓包与解码命令在 Go 服务所在机器执行。手机本地内嵌服务只监听回环地址，使用随机 token。配置的命令具有核心进程的权限，适合受信任的个人分析环境。

## 边界与验证

第一版接收上游重建完成的 HTTP 事件，支持 JSON/HAR 历史导入，不直接解密导入 pcapng。原始 pcapng 应由 eCapture 的 `--pcapfile` 保留；原始 hook 的分片事件不能冒充已重建消息。HTTP/3、QUIC 和 WebSocket 消息级分析尚未实现。业务加密需提供自己的解码器。

eCapture 当前类型化事件时间戳只有秒级精度，因此实时流量的亚秒耗时不能准确计算；导入包含高精度时间戳的事件可以正常计算。当前 HTTP/2 重建事件还可能缺少 `:authority`，这类记录保留原始相对请求目标和目标 IP/端口，不推断域名；需要完整域名时，上游需把 Host/authority 一并输出，或从保留的 pcapng 重新解码。

Android 本地模式使用带通知的前台服务，切到被测 APP 后仍保持核心运行；“Change service” 会停止本地核心和服务。系统强制终止或设备重启后需要重新打开并连接，历史仍在。历史写入为原子 JSON 快照，适合第一版分析量；大 Body 和高吞吐应先测量再替换存储。

```sh
go test -race ./...
go vet ./...
cd ui
flutter analyze
flutter test
```

开发期间已使用 Android 15 Pixel 6 + eCapture v2.9.1，在回环接口生成受控 BoringSSL HTTPS 请求，验证真实 eCapture → Protobuf WebSocket → Go 请求响应配对 → JSON Body 解码。具体构建和检查结果见 [验证记录](docs/verification.md)。

解码链路复用安装的 eCapture，不把 `ecapture_decryptor` SDK 纳入此产品依赖。上游参考：[eCapture 输出管线](https://github.com/gojue/ecapture/blob/v2.9.1/docs/agent/output-pipeline.md)、[v2.9.1 协议](https://github.com/gojue/ecapture/blob/v2.9.1/protobuf/proto/v1/ecaptureq.proto)。

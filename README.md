# LAN Share 0.1.0

MyGo 纯 Go UI 的跨平台 SMB 桌面工具。首版面向 macOS，复用已在极米 NEW Z8X 自带资源管理器验证过的 `go-filesystems/smb v0.7.0`。不依赖 macOS 文件共享服务。

## 使用

1. 打开构建生成的 `dist/LAN Share.app`，选择希望共享的媒体文件夹。
2. 设置 SMB 用户名和密码。默认用户名为 `projector`，未记住密码时每次打开应用会生成新密码；点击“显示”可查看，也可以自行输入。
3. 可选“记住密码”，使用系统凭据存储。点击“保存设置”或“启动共享”时保存。
4. 点击“启动共享”。若当前 macOS 要求 445 端口权限，会出现系统管理员授权窗口。这里输入的是 Mac 管理员密码，与投影仪使用的 SMB 密码不同。
5. 在同一局域网的投影仪资源管理器中填写应用展示的 IP、用户名和 SMB 密码，共享名为 `Media`，端口为 `445`。
6. 点击“停止共享”或关闭应用窗口会停止服务、断开连接并释放端口。运行期间保持电脑唤醒。

只共享选定文件夹，投影仪只能读取；不提供写入、删除、转码或自动复制。修改共享目录或账号密码前需要停止服务。默认仅展示常见私有局域网 IPv4 地址，过滤隧道和常见虚拟网卡；网络变更后重新启动共享可刷新地址。当前使用手动 IP 接入。

服务监听所有本机 IPv4 接口的 445 端口；界面中的地址筛选只影响显示，不限制监听接口。

## macOS 端口授权

GUI 和文件读取都以当前用户身份运行。一个独立的纯 Go 小程序在系统授权后仅绑定 `0.0.0.0:445`，通过私有 Unix socket 传递监听文件描述符，然后立即退出。不会安装常驻服务、修改防火墙或关闭系统 SMB。

助手检查临时目录/控制 socket 的权限、所有者和连接进程 UID；GUI 验证对端为 root 及返回的端口。停止后应用关闭最终的监听描述符。取消或超时会关闭控制通道；助手无法联系应用时不绑定端口。

这是本地测试版，使用 ad-hoc 签名，尚未做 Developer ID 签名和 Apple 公证。最低声明 macOS 13；实际运行验证以 [验证记录](docs/validation.md) 为准。

## 配置和日志

- macOS：`~/Library/Application Support/LAN Share/`
- Windows：用户配置目录下的 `LAN Share/`
- Linux：用户配置目录下的 `LAN Share/`，遵循 `XDG_CONFIG_HOME`
- `settings.json` 仅保存文件夹、用户名和记住密码选项。
- 勾选记住密码时，密码保存在 Keychain / Credential Manager / Secret Service；没有可用凭据存储时提示取消勾选。
- `logs/service.jsonl` 为诊断日志，约 1 MiB 轮转，最多保留三份。
- 日志不记录密码、认证令牌、文件内容或完整文件名，包含局域网 IP、协议状态及文件路径短散列。

诊断日志按钮打开日志文件夹。为避免误共享应用配置，不能选择包含该应用配置目录的上层目录。

## 开发和构建

依赖固定为 MyGo v0.6.3、SMB v0.7.0；Go 1.27.2，无需 Node/Bun。macOS 构建需要 Command Line Tools 中的 `lipo`、`codesign`、`ditto`。

```sh
go test -race ./...
go vet ./...
sh scripts/build-macos.sh
```

脚本默认输出到项目内的 `dist/` 目录（已加入 Git 忽略规则），生成 Apple Silicon + Intel 通用应用和 ZIP。也可以指定输出目录：

```sh
sh scripts/build-macos.sh /absolute/output/directory
```

源码运行 UI：

```sh
go run ./cmd/lan-share
```

`go run` 的临时目录没有端口助手，因此实际 445 授权测试应使用完整 `.app`。`LAN_SHARE_DATA_DIR` 可指定隔离的测试配置目录，凭据服务名称也会隔离。开发过程中不会自动启动共享。

Windows/Linux 保持独立的平台端口实现，Go SMB 核心不依赖 GUI：

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags mygo_noinspector -ldflags '-s -w -H=windowsgui' -o build/lan-share.exe ./cmd/lan-share
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags mygo_noinspector -ldflags '-s -w' -o build/lan-share-linux ./cmd/lan-share
```

Windows 的系统 SMB 可能占用 445；Linux 的 445 可能需要端口绑定能力，本版仅提供明确错误提示，尚未包含 Linux 授权安装器。Linux GUI 需要 GTK 3，记住密码另需 libsecret 和可用 Secret Service。交叉编译通过不等于对应平台真机验收完成。

## 代码布局

- `cmd/lan-share`：MyGo 界面、目录选择、系统凭据和退出流程。
- `internal/smbserver`：只读文件系统、SMB 生命周期、脱敏协议日志和诊断轮转。
- `internal/portbind`：平台监听策略，以及 macOS socket 交接。
- `cmd/port-helper`：macOS 一次性端口授权助手，不处理文件或密码。
- `internal/preferences`：不包含密码的配置持久化。
- `scripts`：macOS 打包、图标生成、第三方许可证收集。
- `docs`：已通过的验证项目及尚待真机验收的范围。
- `dist`：本机生成的应用和安装包，不提交到 Git。

图标为本项目生成的几何图形；第三方许可证见 `THIRD_PARTY_NOTICES.md` 和 `licenses/`。

更新第三方许可证清单（需要 Python 3）：

```sh
go mod download all
python3 scripts/collect-licenses.py
```

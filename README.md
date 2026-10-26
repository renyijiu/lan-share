# LAN Share

[![CI](https://github.com/renyijiu/lan-share/actions/workflows/ci.yml/badge.svg)](https://github.com/renyijiu/lan-share/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/renyijiu/lan-share?include_prereleases)](https://github.com/renyijiu/lan-share/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

把电脑上的一个文件夹以只读 SMB 共享给局域网里的投影仪、电视盒子等设备。界面基于纯 Go 的 [MyGo](https://github.com/egoist/mygo)，SMB 服务基于 [go-filesystems/smb](https://github.com/go-filesystems/smb)，不依赖系统自带的文件共享服务。

目前主要在 macOS 上开发和验证，Windows、Linux 提供可运行的构建；已测试设备和待验收范围见 [验证记录](docs/validation.md)。

## 使用

1. 从 [GitHub Releases](https://github.com/renyijiu/lan-share/releases) 下载对应平台压缩包，或按下文自行构建。macOS 打开 `LAN Share.app`，Windows 打开 `lan-share.exe`，Linux 运行 `./lan-share`，选择希望共享的媒体文件夹。
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

当前 macOS 包使用 ad-hoc 签名，尚未做 Developer ID 签名和 Apple 公证。首次打开如被系统拦截，可在 Finder 中右键应用选择“打开”。请先把应用移到“应用程序”文件夹再使用，原因见 [安全策略](SECURITY.md#已知限制)。最低声明 macOS 13；实际运行验证以 [验证记录](docs/validation.md) 为准。

## 配置和日志

- macOS：`~/Library/Application Support/LAN Share/`
- Windows：用户配置目录下的 `LAN Share/`
- Linux：用户配置目录下的 `LAN Share/`，遵循 `XDG_CONFIG_HOME`
- `settings.json` 仅保存文件夹、用户名和记住密码选项。
- 勾选记住密码时，密码保存在 Keychain / Credential Manager / Secret Service；没有可用凭据存储时提示取消勾选。
- `logs/service.jsonl` 为诊断日志，约 1 MiB 轮转，最多保留三份。
- 日志不记录密码、认证令牌、文件内容或完整文件名，包含局域网 IP、协议状态、文件大小，以及用每次启动随机生成的密钥计算的文件路径短标识。

诊断日志按钮打开日志文件夹。为避免误共享应用配置，不能选择包含该应用配置目录的上层目录。

## 开发和构建

依赖固定为 MyGo v0.6.3、SMB v0.7.0；Go 版本以 `go.mod` 为准，目前为 1.27.2。打包使用 Python 3.11+，无需 Node/Bun。macOS 打包需要在 Mac 上运行，并安装 Command Line Tools 中的 `lipo`、`codesign`、`ditto`。

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
sh scripts/build-macos.sh
```

脚本默认输出到项目内的 `dist/` 目录（已加入 Git 忽略规则），生成 Apple Silicon + Intel 通用应用和带版本号的 ZIP。也可以指定输出目录：

```sh
sh scripts/build-macos.sh /absolute/output/directory
```

源码运行 UI：

```sh
go run ./cmd/lan-share
```

`go run` 的临时目录没有端口助手，因此实际 445 授权测试应使用完整 `.app`。`LAN_SHARE_DATA_DIR` 可指定隔离的测试配置目录，凭据服务名称也会隔离。开发过程中不会自动启动共享。

Windows/Linux 保持独立的平台端口实现，Go SMB 核心不依赖 GUI。统一打包脚本可以在 macOS、Windows 或 Linux 上交叉构建这两个平台：

```sh
python3 scripts/build.py --platform windows
python3 scripts/build.py --platform linux
```

Windows 中可把 `python3` 换成 `python`。本地构建默认使用 `VERSION` 文件，也可以传 `--version 0.2.0`；版本号会写入应用界面和框架元数据。`go run` 使用 `dev` 版本。所有发布二进制使用 `CGO_ENABLED=0`；竞态测试需要系统 C 编译器。

Windows 的系统 SMB 可能占用 445；Linux 的 445 可能需要端口绑定能力，本版仅提供明确错误提示，尚未包含 Linux 授权安装器。Linux GUI 需要 GTK 3，记住密码另需 libsecret 和可用 Secret Service。交叉编译通过不等于对应平台真机验收完成。

## GitHub Actions 与发布

[工作流](.github/workflows/ci.yml) 在 `main` 推送、PR、`v*` tag 推送和手动触发时运行。检查 job 执行 `gofmt`、`go mod tidy -diff`、按三个目标系统运行的 staticcheck、govulncheck，并确认第三方许可证清单与依赖一致。macOS、Windows、Linux 各自在 GitHub 托管 runner 上执行 `go vet`、`go test -race`、发布脚本测试和构建；普通构建的压缩包保留在 Actions Artifacts 14 天。Dependabot 每周检查 Go 依赖和 Actions 更新。许可证清单不记录版本号（版本以 `go.mod` 为准），普通升级无需改动；新增或移除依赖、或依赖的许可证文本变化时，需要重新生成清单才能通过检查。

只有推送 `v*` tag 才发布 Release，且必须等检查和三平台构建全部成功。Go 工具链从 `go.mod` 读取，应用与压缩包版本以 tag 为准。普通 CI 构建使用 `VERSION-dev.<commit>`。

例如要发布 `v0.1.0`，在需要发布的提交上执行：

```sh
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

Tag 必须是带 `v` 前缀的语义版本，如 `v0.1.0` 或 `v0.2.0-rc.1`。带预发布标识的 tag 会生成 Pre-release。发布包含：

- `LAN-Share-v<版本>-macOS-universal.zip`：Intel + Apple Silicon 通用应用。
- `LAN-Share-v<版本>-Windows-amd64.zip`：Windows x64 便携包。
- `LAN-Share-v<版本>-Linux-amd64.tar.gz`：Linux x64 便携包，保留可执行权限。
- `SHA256SUMS.txt`：三个压缩包的 SHA-256 校验值。

工作流先创建草稿，上传完整产物后再发布；失败可重跑，已公开的同名 Release 不覆盖产物。自动生成发布说明。只使用仓库自动提供的 `GITHUB_TOKEN`，发布 job 单独申请 `contents: write`，无需手动添加 PAT。Actions 固定到完整提交 SHA。

macOS 包目前只有 ad-hoc 签名，未做 Apple 公证；Windows 包未做代码签名。Linux 运行依赖仍需用户系统提供。

本地验证发布包与校验文件、不创建 Release：

```sh
python3 scripts/publish.py --tag v0.1.0 --directory dist --dry-run
```

该命令要求 `dist/` 中已经存在同版本的三个平台压缩包。

## 代码布局

- `cmd/lan-share`：MyGo 界面、目录选择、系统凭据和退出流程。
- `internal/smbserver`：只读文件系统、SMB 生命周期、脱敏协议日志和诊断轮转。
- `internal/portbind`：平台监听策略，以及 macOS socket 交接。
- `cmd/port-helper`：macOS 一次性端口授权助手，不处理文件或密码。
- `internal/preferences`：不包含密码的配置持久化。
- `scripts`：macOS 打包、图标生成、第三方许可证收集。
- `docs`：已通过的验证项目及尚待真机验收的范围。
- `dist`：本机生成的应用和安装包，不提交到 Git。

图标为本项目生成的几何图形；本项目许可证见 `LICENSE`，第三方许可证见 `THIRD_PARTY_NOTICES.md` 和 `licenses/`。

更新第三方许可证清单（需要 Python 3）。脚本会下载完整模块图、重新生成 `licenses/`，并保持 `go.sum` 整洁：

```sh
python3 scripts/collect-licenses.py
```

## 安全

请不要在公开 Issue 中报告漏洞，提交方式和已知限制见 [SECURITY.md](SECURITY.md)。

## 许可证

本项目以 [MIT 许可证](LICENSE) 发布。第三方依赖的许可证见 `THIRD_PARTY_NOTICES.md` 和 `licenses/`。

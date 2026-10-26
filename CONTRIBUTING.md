# 参与贡献

感谢你愿意改进 LAN Share！欢迎提交 Bug、设备兼容性反馈、文档修正和代码。Issue 和 PR 用中文或英文都可以。

> English speakers are welcome too: issues and pull requests may be written in English.

## 提问与反馈

- **Bug**：使用 [Bug 报告模板](https://github.com/renyijiu/lan-share/issues/new?template=bug_report.yml)，写明系统版本、应用版本和复现步骤。
- **设备兼容性**：无论你的投影仪、电视盒子或播放器能不能用，都欢迎通过 [设备兼容性模板](https://github.com/renyijiu/lan-share/issues/new?template=device_compat.yml) 告诉我们，结果会整理进 [已测试设备](docs/validation.md#已测试设备)。
- **安全漏洞**：请不要公开提交，按 [SECURITY.md](SECURITY.md) 私下报告。
- 提交前请先看 [常见问题](docs/faq.md)，并搜索是否已有相同的 Issue。

附上诊断日志前请先检查内容：日志不含密码、文件内容或文件名，但包含局域网 IP 地址。

## 开发环境

- Go：版本以 `go.mod` 为准。CI 设置了 `GOTOOLCHAIN=local`，请安装对应版本。
- Python 3.11+：用于打包和许可证收集脚本。
- 竞态测试（`-race`）需要系统 C 编译器；发布二进制使用 `CGO_ENABLED=0`。
- macOS 打包需要 Command Line Tools 中的 `lipo`、`codesign`、`ditto`。
- Linux 运行 GUI 需要 GTK 3，“记住密码”需要 libsecret 和可用的 Secret Service。

从源码运行：

```sh
go run ./cmd/lan-share
```

建议用 `LAN_SHARE_DATA_DIR` 指定一个单独的测试配置目录，这样不会影响你日常使用的设置和已保存的密码：

```sh
LAN_SHARE_DATA_DIR=/tmp/lan-share-dev go run ./cmd/lan-share
```

`go run` 生成的临时目录里没有 macOS 端口助手。要测试 445 端口授权，请用 `sh scripts/build-macos.sh` 构建完整的 `.app`。

## 提交 PR 前

请在本地运行与 CI 相同的检查：

```sh
gofmt -l .                      # 应无输出
go mod tidy -diff
go vet ./...
go test -race ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
```

CI 还会运行 staticcheck（linux、darwin、windows 三个目标）和 govulncheck，命令见 [工作流](.github/workflows/ci.yml)。

- **新增或移除依赖**：运行 `python3 scripts/collect-licenses.py` 并提交 `licenses/` 和 `THIRD_PARTY_NOTICES.md`。普通版本升级不需要。新依赖的许可证必须与 MIT 兼容。
- **涉及以下内容的改动请特别说明并补充测试**：共享目录边界（路径、符号链接）、只读保证、认证、端口助手、日志脱敏。
- **日志**：不要记录密码、认证令牌、文件内容或文件名；文件路径用 `pathID` 生成的标识代替。
- **界面文案**：目前为中文，新增文案请保持一致。界面行为可以用 MyGo 的 `ui.NewTester` 写无窗口测试，参考 `cmd/lan-share/main_test.go`。
- **跨平台**：Windows 和 Linux 尚未全面真机验收。如果你在这两个平台上验证过，请在 PR 中写明系统版本。

## 提交信息

沿用 [Conventional Commits](https://www.conventionalcommits.org/) 风格，常用前缀：

| 前缀 | 用途 |
| --- | --- |
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 只改文档 |
| `refactor` | 不改变行为的重构 |
| `test` | 测试 |
| `build` | 构建、打包、许可证脚本 |
| `ci` | GitHub Actions |
| `deps` | 依赖升级 |
| `chore` | 其他杂项 |

一个 PR 只做一件事。用户能感知到的改动，请同时在 [CHANGELOG.md](CHANGELOG.md) 的 `Unreleased` 下补一条。

## 许可

提交贡献即表示你同意以本项目的 [MIT 许可证](LICENSE) 发布你的代码。

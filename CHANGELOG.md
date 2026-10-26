# 更新日志

本文件记录用户能感知到的改动，格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [0.1.0] - 2026-10-26

首个公开版本。

### 新增

- 把选定文件夹以只读 SMB 共享（共享名 `Media`，端口 445）提供给局域网中的投影仪等设备，支持 SMB 2.0.2 至 3.1.1。
- 用户名和密码认证。默认用户名为 `projector`，未记住密码时每次打开应用会生成随机密码。
- “记住密码”，保存在系统凭据存储中（Keychain / Credential Manager / Secret Service）。
- 列出本机的局域网 IPv4 地址，可一键复制。
- macOS 一次性端口授权助手：只绑定 445 并把监听交还给应用，不安装常驻服务、不修改防火墙。
- 脱敏诊断日志，约 1 MiB 轮转，最多保留三份。
- 提供 macOS 通用包（Apple Silicon + Intel）、Windows x64 和 Linux x64 便携包，附带 SHA-256 校验文件。

### 安全

- 共享目录之外的文件（包括通过符号链接、`..` 或备用数据流访问的）不可读取；所有写入、删除和重命名都会被拒绝。
- 日志中的文件路径标识使用每次启动随机生成的密钥计算。
- 端口助手只接受规范化后的控制 socket 路径。

### 致预览版测试者

- 应用标识改为 `io.github.renyijiu.lanshare`。用之前的预览构建保存过密码的，需要重新输入一次。

[Unreleased]: https://github.com/renyijiu/lan-share/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/renyijiu/lan-share/releases/tag/v0.1.0

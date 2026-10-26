# LAN Share

[![CI](https://github.com/renyijiu/lan-share/actions/workflows/ci.yml/badge.svg)](https://github.com/renyijiu/lan-share/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/renyijiu/lan-share?include_prereleases)](https://github.com/renyijiu/lan-share/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[简体中文](README.md) | English

Share one folder on your computer, read-only over SMB, with projectors, TV boxes and media players on your local network. The UI is built with the pure-Go [MyGo](https://github.com/egoist/mygo) toolkit and the server with [go-filesystems/smb](https://github.com/go-filesystems/smb). It does not use or change the operating system's own file sharing.

Development and testing happen mainly on macOS. Windows and Linux builds run, but have not been fully verified on real hardware. See the [validation record](docs/validation.md) for tested devices. The user interface is currently in Chinese only.

## Features

- Read-only share named `Media` on port 445, speaking SMB 2.0.2 through 3.1.1.
- Username and password sign-in. The default username is `projector`; a random password is generated on every launch unless you choose to remember one.
- Optional "remember password" backed by Keychain, Credential Manager or Secret Service.
- Lists the computer's LAN IPv4 addresses, ready to copy.
- On macOS, a one-shot helper binds port 445 after administrator approval and hands it back to the app. Nothing is installed, and the firewall is not touched.
- Redacted diagnostic logs: no passwords, file contents or file names.

## Usage

1. Download the package for your platform from [GitHub Releases](https://github.com/renyijiu/lan-share/releases), or build it yourself as described below. On macOS, move `LAN Share.app` to Applications first. On Windows, run `lan-share.exe`; on Linux, run `./lan-share`.
2. Choose the media folder to share.
3. Set the SMB username and password. Click the eye icon to reveal the password, or type your own.
4. Optionally turn on "记住密码" (remember password). Settings are saved when you click "保存设置" (save) or "启动共享" (start sharing).
5. Click "启动共享" (start sharing). If macOS needs permission for port 445, it asks for your **Mac administrator password**. This is not the SMB password used on the projector.
6. On the projector, add an SMB server in its file manager with the IP address shown in the app, port `445`, share `Media`, and the username and password from the app.
7. Click "停止共享" (stop sharing) or close the window to stop the server, disconnect clients and release the port. Keep the computer awake while sharing; the app does not prevent sleep.

Only the chosen folder is shared, and it is always read-only. Nothing is written, deleted, transcoded or copied. Stop the share before changing the folder or the credentials. The server listens on port 445 of every local IPv4 interface; the address list in the app is only a hint and does not restrict which interfaces accept connections.

### Opening the app on macOS

Builds are ad-hoc signed and not yet notarized, so macOS blocks the first launch. Open the app once, dismiss the warning, then go to **System Settings → Privacy & Security** and click **Open Anyway**. On macOS 14 and earlier, you can also Control-click the app in Finder and choose **Open**. The minimum declared version is macOS 13.

### How port authorization works on macOS

The UI and all file access run as the current user. A separate, minimal Go helper runs once with administrator approval. It binds only `0.0.0.0:445`, passes the listening socket to the app over a private Unix socket and exits. The helper checks the permissions and owner of its control directory and socket and the peer's UID; the app checks that the peer is root and that the port is the expected one. Canceling or timing out closes the control channel, and the helper binds nothing if it cannot reach the app.

## Troubleshooting

The [FAQ](docs/faq.md) (in Chinese) covers these in more detail.

- **"445 端口已被占用" (port 445 is in use):** another SMB server holds the port. On macOS, turn off File Sharing in System Settings → General → Sharing. On Windows, the built-in Server service uses 445 by default. On Linux, stop Samba (`sudo systemctl stop smbd`).
- **Linux cannot listen on 445:** grant the binary permission to bind low ports with `sudo setcap 'cap_net_bind_service=+ep' ./lan-share`. Repeat after every update.
- **The projector cannot connect:** make sure both devices are on the same network, not a guest network, and that the router's AP or client isolation is off. Allow incoming connections if the firewall asks.
- **The device only speaks SMB1:** SMB1 is not supported. Look for an SMB2 option on the device, or use a player app that supports SMB2, such as Kodi or VLC.
- **Windows SmartScreen warning:** the Windows build is not code-signed yet. Click "More info → Run anyway".

## Configuration and logs

| Platform | Location |
| --- | --- |
| macOS | `~/Library/Application Support/LAN Share/` |
| Windows | `%AppData%\LAN Share\` |
| Linux | `~/.config/LAN Share/` (honors `XDG_CONFIG_HOME`) |

- `settings.json` stores only the folder, the username and the "remember password" choice, never the password.
- Remembered passwords live in the system credential store under `io.github.renyijiu.lanshare`.
- `logs/service.jsonl` is the diagnostic log. It rotates at about 1 MiB and keeps three files. It contains LAN IP addresses, protocol states, file sizes and short path IDs keyed by a random per-launch secret, but no passwords, tokens, file contents or file names. The "诊断日志" (diagnostic log) button opens the folder.
- To avoid sharing the app's own data, you cannot pick a folder that contains the configuration directory.

## Building from source

The Go version is pinned in `go.mod`. Packaging needs Python 3.11+. macOS packages must be built on a Mac with the Command Line Tools (`lipo`, `codesign`, `ditto`). Race tests need a C compiler; release binaries use `CGO_ENABLED=0`.

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
sh scripts/build-macos.sh                      # macOS universal app and ZIP in dist/
python3 scripts/build.py --platform windows    # cross-builds from any host
python3 scripts/build.py --platform linux
```

Run the UI from source with `go run ./cmd/lan-share`. Set `LAN_SHARE_DATA_DIR` to use a separate configuration directory and credential entry while developing. `go run` has no port helper, so test port 445 authorization with the full `.app`. The Linux GUI needs GTK 3; remembering passwords also needs libsecret and a running Secret Service.

Releases are published by CI when a `v*` tag such as `v0.1.0` is pushed, after all checks and platform builds pass. Details are in the [Chinese README](README.md#github-actions-与发布).

## Code layout

- `cmd/lan-share`: MyGo UI, folder picker, credential storage and shutdown.
- `cmd/port-helper`: the one-shot macOS port helper; it never touches files or passwords.
- `internal/smbserver`: read-only filesystem, SMB lifecycle, redacted protocol trace and log rotation.
- `internal/portbind`: per-platform listening, and the macOS socket handoff.
- `internal/preferences`: settings persistence, without passwords.
- `scripts`: packaging, icon generation and third-party license collection.
- `docs`: FAQ and the validation record.

## Contributing

Bug reports, device compatibility reports and pull requests are welcome, in English or Chinese. See [CONTRIBUTING.md](CONTRIBUTING.md) and the [changelog](CHANGELOG.md).

## Security

Please do not report vulnerabilities in public issues. See [SECURITY.md](SECURITY.md) for how to report them privately and for known limitations.

## License

LAN Share is released under the [MIT License](LICENSE). Third-party licenses are listed in `THIRD_PARTY_NOTICES.md` and `licenses/`.

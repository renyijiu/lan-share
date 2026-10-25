package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"local/lan-share/internal/portbind"
	"local/lan-share/internal/preferences"
	"local/lan-share/internal/smbserver"
)

const version = "0.1.0"

type application struct {
	win          *mygo.Window
	controller   *smbserver.Controller
	ctx          context.Context
	cancel       context.CancelFunc
	cancelStart  context.CancelFunc
	startDone    chan struct{}
	settings     preferences.Settings
	password     string
	showPassword bool
	dir          string
	secrets      mygo.SecretsModule
	snapshot     smbserver.Snapshot
	addresses    []string
	action       string
	message      string
	messageError bool
	quitting     bool
	quitReady    bool
}

func main() {
	mygo.App.SetName("LAN Share")
	mygo.App.SetVersion(version)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	dir, err := preferences.Directory()
	if err != nil {
		log.Fatal(err)
	}
	logger, err := smbserver.OpenLog(filepath.Join(dir, "logs"))
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := "dev.lanshare.desktop"
	if os.Getenv("LAN_SHARE_DATA_DIR") != "" {
		sum := sha256.Sum256([]byte(dir))
		service += ".test-" + hex.EncodeToString(sum[:6])
	}
	a := &application{dir: dir, ctx: ctx, cancel: cancel, secrets: mygo.Secrets.Service(service), snapshot: smbserver.Snapshot{State: smbserver.Stopped}, action: "loading"}
	a.controller = smbserver.New(logger, portbind.Listen)
	defer a.controller.Stop()
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}, {Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu}}))
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "LAN Share · 局域网共享", Width: 740, Height: 750, MinWidth: 620, MinHeight: 620, Content: ui.View(a.view)})
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitReady {
				e.PreventDefault()
				a.quit()
			}
		})
		mygo.App.OnSecondInstance(func([]string, string) { a.win.Show(); a.win.Focus() })
		go a.load()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-a.controller.Changed:
					a.win.Update(func() { a.snapshot = a.controller.Snapshot() })
				}
			}
		}()
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !a.quitReady {
			e.PreventDefault()
			a.quit()
		}
	})
	mygo.App.OnWindowAllClosed(func() { mygo.App.Quit() })
	if err = mygo.App.Run(); err != nil {
		log.Print(err)
	}
}

func (a *application) load() {
	s, err := preferences.Load(a.dir)
	var password, message string
	if err != nil {
		s = preferences.Settings{Username: "projector"}
		message = "未能读取上次配置，请重新设置。"
	}
	if s.RememberPassword {
		p, e := a.secrets.Get("smb-password")
		if e != nil {
			message = "无法读取已保存的密码，请重新输入。"
			s.RememberPassword = false
		} else {
			password = string(p)
			clear(p)
		}
	} else {
		var b [6]byte
		if _, e := rand.Read(b[:]); e == nil {
			password = hex.EncodeToString(b[:])
		}
	}
	addresses := smbserver.Addresses("445")
	a.win.Update(func() {
		a.settings = s
		a.password = password
		a.message = message
		a.messageError = message != ""
		a.addresses = addresses
		a.action = ""
	})
}

func (a *application) config() smbserver.Config {
	return smbserver.Config{Folder: a.settings.Folder, Username: a.settings.Username, Password: a.password}
}

func (a *application) saveSettings(s preferences.Settings, password string) error {
	// Do not put configuration or diagnostics inside the selected share.
	root, e := filepath.EvalSymlinks(s.Folder)
	data, e2 := filepath.EvalSymlinks(a.dir)
	if e == nil && e2 == nil {
		rel, e := filepath.Rel(root, data)
		if e == nil && (rel == "." || filepath.IsLocal(rel)) {
			return errors.New("所选目录包含应用配置和日志，请选择更具体的媒体文件夹")
		}
	}
	if s.RememberPassword {
		if err := a.secrets.Set("smb-password", []byte(password)); err != nil {
			return errors.New("系统凭据存储不可用，请取消“记住密码”后重试")
		}
	} else {
		if err := a.secrets.Delete("smb-password"); err != nil && !errors.Is(err, errors.ErrUnsupported) {
			return errors.New("无法清除已保存的密码，请解锁系统凭据存储后重试")
		}
	}
	if err := preferences.Save(a.dir, s); err != nil {
		return errors.New("无法保存设置，请检查应用配置目录的写入权限")
	}
	return nil
}

func (a *application) start() {
	if a.action != "" || a.quitting {
		return
	}
	cfg, s := a.config(), a.settings
	a.message = ""
	a.messageError = false
	a.action = "starting"
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelStart = cancel
	done := make(chan struct{})
	a.startDone = done
	go func() {
		defer close(done)
		err := cfg.Validate()
		if err == nil {
			err = a.saveSettings(s, cfg.Password)
		}
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = a.controller.Start(ctx, cfg)
		}
		if err != nil {
			cancel()
		}
		addresses := smbserver.Addresses("445")
		a.win.Update(func() {
			if a.action == "starting" {
				a.action = ""
			}
			a.snapshot = a.controller.Snapshot()
			a.addresses = addresses
			if err != nil && !errors.Is(err, context.Canceled) {
				a.message = err.Error()
				a.messageError = true
			}
		})
	}()
}

func (a *application) stop() {
	if a.action == "stopping" {
		return
	}
	if a.cancelStart != nil {
		a.cancelStart()
		a.cancelStart = nil
	}
	a.action = "stopping"
	a.message = ""
	a.messageError = false
	done := a.startDone
	go func() {
		if done != nil {
			<-done
		}
		a.controller.Stop()
		a.win.Update(func() { a.action = ""; a.snapshot = a.controller.Snapshot() })
	}()
}

func (a *application) save() {
	if a.action != "" {
		return
	}
	cfg, s := a.config(), a.settings
	a.action = "saving"
	go func() {
		err := cfg.Validate()
		if err == nil {
			err = a.saveSettings(s, cfg.Password)
		}
		a.win.Update(func() {
			a.action = ""
			a.messageError = err != nil
			if err != nil {
				a.message = err.Error()
			} else {
				a.message = "设置已保存"
			}
		})
	}()
}

func (a *application) pickFolder() {
	if a.action != "" {
		return
	}
	a.action = "picking"
	initial := a.settings.Folder
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择共享文件夹", Directory: true, DefaultPath: initial})
		a.win.Update(func() {
			a.action = ""
			if err != nil {
				a.message = "无法打开文件夹选择器"
				a.messageError = true
			} else if len(paths) > 0 {
				a.settings.Folder = paths[0]
				a.message = ""
				a.messageError = false
			}
		})
	}()
}

func (a *application) quit() {
	if a.quitting {
		return
	}
	a.quitting = true
	a.cancel()
	done := a.startDone
	go func() {
		if done != nil {
			<-done
		}
		a.controller.Stop()
		a.win.Update(func() { a.quitReady = true; mygo.App.Quit() })
	}()
}

func (a *application) view(c *ui.Context) {
	theme := *c.Theme()
	theme.Accent = ui.Hex("#147d75")
	theme.Radius = 8
	c.SetTheme(&theme)
	t := c.Theme()
	running := a.snapshot.State == smbserver.Running
	locked := running || a.action != "" || a.snapshot.State == smbserver.Stopping || a.quitting
	ui.Scroll(c).Fill().Padding(28).Gap(20).Background(t.Background).Children(func() {
		ui.Row(c).Gap(14).Children(func() {
			ui.Box(c).Size(48, 48).Radius(13).Background(t.Accent).Center().Children(func() { ui.Text(c, "LAN").FontSize(15).Bold().TextColor(ui.Hex("#ffffff")) })
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "局域网共享").FontSize(25).Bold()
				ui.Text(c, "把电脑上的文件分享给投影仪").TextColor(t.TextMuted)
			})
			ui.Text(c, "v"+version).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Column(c).Padding(20).Gap(10).Radius(12).Background(t.Surface).Border(1, t.Border).Children(func() {
			label, color := "尚未启动", t.TextMuted
			if running {
				label, color = "共享中", ui.Hex("#14856a")
			}
			if a.action == "starting" || a.snapshot.State == smbserver.Starting {
				label, color = "正在启动…", t.Accent
			}
			if a.action == "stopping" || a.snapshot.State == smbserver.Stopping {
				label = "正在停止…"
			}
			if a.snapshot.State == smbserver.Failed {
				label, color = "启动未完成", t.Danger
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(color)
				ui.Text(c, label).Bold().TextColor(color)
				ui.Spacer(c)
				ui.Text(c, "SMB · 只读共享").FontSize(12).TextColor(t.TextMuted)
			})
			if running {
				ui.Textf(c, "%d 个连接 · 停止共享将断开正在播放的设备", a.snapshot.Connections).FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "选择文件夹并设置登录信息，然后启动共享。").FontSize(12).TextColor(t.TextMuted)
			}
		})
		ui.Column(c).Gap(10).Children(func() {
			ui.Text(c, "共享文件夹").Bold()
			ui.Row(c).Gap(10).Children(func() {
				ui.TextInput(c.Key("folder"), &a.settings.Folder).ReadOnly(true).Placeholder("尚未选择文件夹").Label("共享文件夹路径").Grow(1).Height(40)
				if ui.Button(c.Key("choose-folder"), "选择文件夹…").Height(40).Disabled(locked).Clicked() {
					a.pickFolder()
				}
			})
			ui.Text(c, "共享名 Media · 投影仪只能浏览和读取文件").FontSize(12).TextColor(t.TextMuted)
		})
		ui.Column(c).Gap(12).Children(func() {
			ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
				ui.Field(c, "用户名", func() { ui.TextInput(c.Key("username"), &a.settings.Username).Disabled(locked).Height(40) }).Width(210)
				ui.Field(c, "密码", func() {
					ui.Row(c).Gap(8).Children(func() {
						p := ui.TextInput(c.Key("password"), &a.password).Disabled(locked).Grow(1).Height(40)
						if !a.showPassword {
							p.Password()
						}
						label := "显示"
						if a.showPassword {
							label = "隐藏"
						}
						if ui.Button(c.Key("show-password"), label).Height(40).Clicked() {
							a.showPassword = !a.showPassword
						}
					})
				}).Grow(1)
			})
			ui.Checkbox(c.Key("remember-password"), &a.settings.RememberPassword, "记住密码（保存在系统凭据存储中）").Disabled(locked)
		})
		ui.Row(c).Gap(10).Children(func() {
			if a.action == "starting" || a.snapshot.State == smbserver.Starting {
				if ui.Button(c.Key("cancel-start"), "取消启动").Height(42).Width(140).Clicked() {
					a.stop()
				}
			} else if running {
				if ui.Button(c.Key("stop"), "停止共享").Height(42).Width(140).Disabled(a.action != "").Clicked() {
					a.stop()
				}
			} else {
				if ui.PrimaryButton(c.Key("start"), "启动共享").Height(42).Width(140).Disabled(locked).Clicked() {
					a.start()
				}
			}
			if ui.Button(c.Key("save"), "保存设置").Height(42).Disabled(locked).Clicked() {
				a.save()
			}
			ui.Spacer(c)
			if ui.Button(c.Key("logs"), "诊断日志").Height(42).Clicked() {
				go mygo.Shell.OpenPath(filepath.Join(a.dir, "logs"))
			}
		})
		message := a.message
		isError := a.messageError
		if message == "" && a.snapshot.Error != "" {
			message = a.snapshot.Error
			isError = true
		}
		if message != "" {
			color := t.Accent
			if isError {
				color = t.Danger
			}
			ui.Text(c, message).TextColor(color).FontSize(13)
		}
		ui.Divider(c)
		ui.Column(c).Gap(10).Children(func() {
			ui.Text(c, "在投影仪上连接").Bold()
			ui.Text(c, "接入同一局域网，在资源管理器中添加地址，填写上面的账号密码。").FontSize(12).TextColor(t.TextMuted)
			if len(a.addresses) == 0 {
				ui.Text(c, "未找到局域网地址，请连接 Wi-Fi 或以太网。").TextColor(t.TextMuted)
			}
			for _, address := range a.addresses {
				ui.Row(c.Key(address)).Gap(8).Children(func() {
					ui.Text(c, address).FontSize(14).Grow(1).Selectable()
					if ui.Button(c, "复制").Label("复制 " + address).Clicked() {
						mygo.Clipboard.WriteText(address)
						a.message = "连接地址已复制"
						a.messageError = false
					}
				})
			}
		})
		hint := "退出应用会停止共享；运行期间请保持电脑唤醒。"
		if runtime.GOOS == "darwin" {
			hint += "启动时 macOS 可能请求端口授权。"
		}
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
	})
}

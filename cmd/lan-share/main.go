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
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "LAN Share", Width: 600, Height: 700, MinWidth: 520, MinHeight: 560, TitleBarStyle: mygo.TitleBarHidden, Content: ui.View(a.view)})
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

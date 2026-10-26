package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/renyijiu/lan-share/internal/smbserver"
)

// palette holds the colors the theme has no slot for: the cards on the
// warm window background, and the projector lamp that lights while sharing.
type palette struct {
	card     ui.Color
	shadow   ui.Color
	edge     ui.Color
	rule     ui.Color
	lamp     ui.Color
	lampText ui.Color
}

// applyTheme sets the app's warm neutrals and amber accent, in the light
// or dark appearance the system asks for.
func applyTheme(c *ui.Context) (*ui.Theme, palette) {
	t := *c.Theme()
	var p palette
	if t.Dark {
		t.Background = ui.Hex("#191816")
		t.Surface = ui.Hex("#2e2d2a")
		t.SurfaceHover = ui.Hex("#373632")
		t.SurfacePressed = ui.Hex("#41403b")
		t.Border = ui.Hex("#3d3b37")
		t.Text = ui.Hex("#f2efe9")
		t.TextMuted = ui.Hex("#9b968d")
		t.Accent = ui.Hex("#f4ad4c")
		t.AccentHover = ui.Hex("#f7bb66")
		t.AccentPressed = ui.Hex("#e79b36")
		t.AccentText = ui.Hex("#251705")
		t.Danger = ui.Hex("#ff7262")
		t.Selection = ui.RGBA(244, 173, 76, 0.32)
		t.Focus = ui.RGBA(244, 173, 76, 0.6)
		p = palette{
			card:     ui.Hex("#22211f"),
			shadow:   ui.RGBA(0, 0, 0, 0.35),
			edge:     ui.RGBA(255, 248, 235, 0.06),
			rule:     ui.RGBA(255, 248, 235, 0.08),
			lamp:     ui.Hex("#f4ad4c"),
			lampText: ui.Hex("#f6c27a"),
		}
	} else {
		t.Background = ui.Hex("#f4f2ee")
		t.Surface = ui.Hex("#f3f1ed")
		t.SurfaceHover = ui.Hex("#ebe8e3")
		t.SurfacePressed = ui.Hex("#e1ded8")
		t.Border = ui.Hex("#dcd8d1")
		t.Text = ui.Hex("#1f1d1a")
		t.TextMuted = ui.Hex("#7a756d")
		t.Accent = ui.Hex("#f2a53f")
		t.AccentHover = ui.Hex("#eb9a2f")
		t.AccentPressed = ui.Hex("#dc8c22")
		t.AccentText = ui.Hex("#2a1a05")
		t.Danger = ui.Hex("#c8412f")
		t.Selection = ui.RGBA(242, 165, 63, 0.3)
		t.Focus = ui.RGBA(226, 140, 30, 0.6)
		t.Inverse = ui.Hex("#2a2825")
		t.InverseText = ui.Hex("#f4f2ee")
		p = palette{
			card:     ui.Hex("#ffffff"),
			shadow:   ui.RGBA(70, 50, 20, 0.07),
			edge:     ui.RGBA(60, 45, 20, 0.07),
			rule:     ui.RGBA(60, 45, 20, 0.09),
			lamp:     ui.Hex("#f2a53f"),
			lampText: ui.Hex("#a8620c"),
		}
	}
	t.Radius = 7
	c.SetTheme(&t)
	return c.Theme(), p
}

// card is a white panel lifted off the window background.
func card(c *ui.Context, p palette) ui.Element {
	return ui.Column(c).Radius(12).Background(p.card).
		Shadow(0, 1, 2, 0, p.shadow).Shadow(0, 6, 18, -6, p.shadow).
		InsetShadow(0, 0, 0, 1, p.edge)
}

// section is a titled group of settings with a note below it.
func section(c *ui.Context, title, note string, body func()) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Padding(0, 4)
		body()
		ui.Text(c, note).FontSize(12).LineHeight(1.45).TextColor(t.TextMuted).Padding(0, 4)
	})
}

// iconButton is a small borderless button showing only an icon, named by
// label for assistive technology and the tooltip.
func iconButton(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(26, 26).Radius(6).Center().Label(label).Tooltip(label)
	switch {
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(15).TextColor(t.TextMuted) })
	return b
}

type tone int

const (
	toneIdle tone = iota
	toneBusy
	toneLive
	toneFailed
)

// beacon is the status lamp: dark while idle, lit and breathing while the
// share is live.
func beacon(c *ui.Context, k tone, p palette) {
	t := c.Theme()
	b := ui.Box(c).Size(44, 44).Shrink(0)
	color := t.TextMuted
	switch k {
	case toneBusy, toneLive:
		color = p.lamp
	case toneFailed:
		color = t.Danger
	}
	phase := float32(-1)
	if (k == toneLive || k == toneBusy) && !c.Preferences().ReduceMotion {
		period := 2400 * time.Millisecond
		if k == toneBusy {
			period = 1100 * time.Millisecond
		}
		phase = b.Loop("pulse", period, ui.Linear)
	}
	b.Draw(func(pt *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		halo := float32(0.1)
		if k == toneLive {
			halo = 0.18
		}
		pt.FillPath((&ui.Path{}).Circle(cx, cy, 21), color.Alpha(halo))
		if phase >= 0 {
			e := 1 - (1-phase)*(1-phase)
			pt.StrokePath((&ui.Path{}).Circle(cx, cy, 7+13*e), 1.5, color.Alpha(0.55*(1-phase)))
		}
		if k == toneIdle {
			pt.StrokePath((&ui.Path{}).Circle(cx, cy, 6), 2, color.Alpha(0.7))
			return
		}
		pt.FillPath((&ui.Path{}).Circle(cx, cy, 7.5), color)
		if k == toneLive {
			pt.FillPath((&ui.Path{}).Circle(cx-2, cy-2, 2.2), ui.RGBA(255, 255, 255, 0.75))
		}
	})
}

// perforation is the torn line between the ticket and its stub, with a
// notch bitten out of either edge.
func perforation(c *ui.Context, p palette) {
	bg := c.Theme().Background
	ui.Box(c).Height(16).Draw(func(pt *ui.Painter, r ui.Rect) {
		y := r.Y + r.H/2
		for x := r.X + 16; x+4 <= r.X+r.W-16; x += 8 {
			pt.Line(x, y, x+4, y, 1, p.rule)
		}
		for _, x := range []float32{r.X, r.X + r.W} {
			const n, k = 8, 8 * 0.5523
			d := float32(1)
			if x > r.X {
				d = -1
			}
			pt.FillPath((&ui.Path{}).Circle(x, y, n), bg)
			bite := (&ui.Path{}).MoveTo(x, y-n).CubeTo(x+d*k, y-n, x+d*n, y-k, x+d*n, y).CubeTo(x+d*n, y+k, x+d*k, y+n, x, y+n)
			pt.StrokePath(bite, 1, p.rule)
		}
	})
}

// shareState is what the share is doing, as the window shows it.
type shareState struct {
	running, starting, stopping bool
	// locked keeps the settings from changing while the share runs or
	// another action is under way.
	locked bool
}

func (a *application) state() shareState {
	s := shareState{
		running:  a.snapshot.State == smbserver.Running,
		starting: a.action == "starting" || a.snapshot.State == smbserver.Starting,
		stopping: a.action == "stopping" || a.snapshot.State == smbserver.Stopping,
	}
	s.locked = s.running || a.action != "" || a.snapshot.State == smbserver.Stopping || a.quitting
	return s
}

// status returns the lamp's tone and the ticket's headline and detail.
func (a *application) status(s shareState) (tone, string, string) {
	switch {
	case a.snapshot.State == smbserver.Failed:
		return toneFailed, "启动未完成", "请查看下方提示后重试。"
	case s.starting:
		if runtime.GOOS == "darwin" {
			return toneBusy, "正在启动…", "macOS 可能会请求端口授权。"
		}
		return toneBusy, "正在启动…", "正在打开 445 端口。"
	case s.stopping:
		return toneBusy, "正在停止…", "正在断开已连接的设备。"
	case s.running:
		if n := a.snapshot.Connections; n > 0 {
			return toneLive, "正在共享", fmt.Sprintf("%d 台设备已连接，停止共享会中断播放。", n)
		}
		return toneLive, "正在共享", "等待投影仪连接。"
	case a.settings.Folder == "":
		return toneIdle, "未在共享", "先选择要共享的文件夹。"
	}
	return toneIdle, "未在共享", "准备就绪，启动后投影仪即可访问。"
}

func (a *application) view(c *ui.Context) {
	t, p := applyTheme(c)
	s := a.state()
	// Confirmations show as toasts; errors stay in the notice until the
	// next action clears them.
	if a.message != "" && !a.messageError {
		c.Toast(a.message)
		a.message = ""
	}
	bar := c.TitleBar()
	side := max(bar.Left, bar.Right) + 16
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Height(max(bar.Height, 40)).Padding(0, side).DragWindow().Children(func() {
			ui.Text(c, "LAN Share").FontSize(13).FontWeight(600).TextColor(t.TextMuted).Grow(1).TextAlign(ui.Center)
		})
		ui.Scroll(c).Grow(1).Padding(2, 24, 22, 24).Gap(20).Children(func() {
			ui.Column(c).Gap(10).Children(func() {
				a.ticket(c, p, s)
				a.notice(c)
			})
			section(c, "共享文件夹", "以共享名 "+smbserver.ShareName+" 只读共享，投影仪只能浏览和播放，无法修改或删除文件。", func() {
				a.folderCard(c, p, s.locked)
			})
			section(c, "登录信息", "投影仪连接时填写这组用户名和密码。记住密码后，密码保存在系统凭据存储中。", func() {
				a.loginCard(c, p, s.locked)
			})
			a.footer(c, p, s.locked)
		})
	})
}

func (a *application) ticket(c *ui.Context, p palette, s shareState) {
	t := c.Theme()
	k, title, detail := a.status(s)
	card(c, p).Children(func() {
		ui.Row(c).Padding(18, 20, 16, 18).Gap(14).Children(func() {
			beacon(c, k, p)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, title).FontSize(17).FontWeight(600)
				ui.Text(c, detail).FontSize(12.5).TextColor(t.TextMuted)
			})
			switch {
			case s.starting:
				if ui.Button(c.Key("cancel-start"), "取消启动").Height(32).Padding(0, 16).Clicked() {
					a.stop()
				}
			case s.running:
				if ui.Button(c.Key("stop"), "停止共享").Height(32).Padding(0, 16).Disabled(a.action != "").Clicked() {
					a.stop()
				}
			default:
				if ui.PrimaryButton(c.Key("start"), "启动共享").Height(32).Padding(0, 18).FontWeight(600).Disabled(s.locked).Clicked() {
					a.start()
				}
			}
		})
		perforation(c, p)
		ui.Column(c).Padding(4, 20, 18, 20).Gap(12).Children(func() {
			ui.Text(c, "在投影仪的文件管理器中添加 SMB 服务器，端口 445").FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).AlignItems(ui.Start).Gap(12).Children(func() {
				stub(c, "地址", 1.5, func() {
					if len(a.addresses) == 0 {
						ui.Text(c, "未找到局域网地址").FontSize(13).TextColor(t.TextMuted)
						ui.Text(c, "请连接 Wi-Fi 或以太网").FontSize(12).TextColor(t.TextMuted)
					}
					for i, address := range a.addresses {
						ui.Row(c.Key(address)).Gap(4).Children(func() {
							v := ui.Text(c, address).FontSize(16).FontWeight(600).FontFeatures("tnum").Selectable()
							if i > 0 {
								v.FontSize(13).FontWeight(400).TextColor(t.TextMuted)
							}
							if iconButton(c, copyIcon, "复制 "+address).Clicked() {
								mygo.Clipboard.WriteText(address)
								c.Toast("已复制 " + address)
							}
						})
					}
				})
				stub(c, "共享名", 1, func() { stubValue(c, smbserver.ShareName) })
				stub(c, "用户名", 1.1, func() {
					name := a.settings.Username
					if name == "" {
						name = "—"
					}
					stubValue(c, name)
				})
			})
		})
	})
}

// stub is one labeled cell of the ticket's stub.
func stub(c *ui.Context, label string, grow float32, fn func()) {
	ui.Column(c).Basis(0).Grow(grow).MinWidth(0).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(11).FontWeight(500).LetterSpacing(0.6).TextColor(c.Theme().TextMuted)
		fn()
	})
}

func stubValue(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(16).FontWeight(600).SingleLine().Ellipsis("…").Selectable()
}

// notice shows what went wrong, under the ticket.
func (a *application) notice(c *ui.Context) {
	t := c.Theme()
	message := a.message
	if message == "" && a.snapshot.Error != "" {
		message = a.snapshot.Error
	}
	if message == "" {
		return
	}
	ui.Row(c).Padding(10, 14).Gap(10).Radius(10).AlignItems(ui.Start).
		Background(t.Danger.Alpha(0.09)).Children(func() {
		ui.Icon(c, alertIcon).FontSize(16).TextColor(t.Danger)
		ui.Text(c, message).FontSize(12.5).TextColor(t.Danger).Grow(1).Selectable()
	})
}

func (a *application) folderCard(c *ui.Context, p palette, locked bool) {
	t := c.Theme()
	card(c, p).Children(func() {
		ui.Row(c).Padding(14, 16).Gap(12).Children(func() {
			ui.Box(c).Size(36, 36).Shrink(0).Radius(9).Center().Background(p.lamp.Alpha(0.16)).Children(func() {
				ui.Icon(c, folderIcon).FontSize(19).TextColor(p.lampText)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				if a.settings.Folder == "" {
					ui.Text(c, "尚未选择文件夹").FontSize(13.5).FontWeight(600).TextColor(t.TextMuted)
					ui.Text(c, "选择存放电影、照片或音乐的文件夹").FontSize(12).TextColor(t.TextMuted)
					return
				}
				ui.Text(c, filepath.Base(a.settings.Folder)).FontSize(13.5).FontWeight(600).SingleLine()
				ui.Text(c, tildePath(filepath.Dir(a.settings.Folder))).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			})
			label := "选择…"
			if a.settings.Folder == "" {
				label = "选择文件夹…"
			}
			if ui.Button(c.Key("choose-folder"), label).Height(30).Padding(0, 14).Disabled(locked).Clicked() {
				a.pickFolder()
			}
		})
	})
}

// tildePath shortens a path inside the home folder to start with ~.
func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

func (a *application) loginCard(c *ui.Context, p palette, locked bool) {
	row := func(e ui.Element) ui.Element {
		return e.Row().Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(16).Padding(9, 16).MinHeight(48)
	}
	card(c, p).Dividers(1, p.rule).Children(func() {
		row(ui.Field(c, "用户名", func() {
			ui.TextInput(c.Key("username"), &a.settings.Username).Disabled(locked).Width(220).Height(30)
		}))
		row(ui.Field(c, "密码", func() {
			ui.Row(c).Gap(4).Children(func() {
				in := ui.TextInput(c.Key("password"), &a.password).Disabled(locked).Width(190).Height(30)
				if !a.showPassword {
					in.Password()
				}
				svg, label := eyeIcon, "显示"
				if a.showPassword {
					svg, label = eyeOffIcon, "隐藏"
				}
				if iconButton(c.Key("show-password"), svg, label).Size(30, 30).Clicked() {
					a.showPassword = !a.showPassword
				}
			})
		}))
		row(ui.Field(c, "记住密码", func() {
			ui.Switch(c.Key("remember-password"), &a.settings.RememberPassword).Disabled(locked)
		}))
	})
}

func (a *application) footer(c *ui.Context, p palette, locked bool) {
	t := c.Theme()
	hint := "退出应用会停止共享，共享期间请保持电脑唤醒。"
	if runtime.GOOS == "darwin" {
		hint += "启动时 macOS 可能请求 445 端口授权，需要输入的是 Mac 管理员密码。"
	}
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("save"), "保存设置").Height(30).Padding(0, 14).Background(p.card).Disabled(locked).Clicked() {
				a.save()
			}
			ui.Spacer(c)
			logs := ui.Button(c.Key("logs"), "").Height(30).Padding(0, 12).Gap(6).Background(p.card).Children(func() {
				ui.Icon(c, logIcon).FontSize(15).TextColor(t.TextMuted)
				ui.Text(c, "诊断日志").SingleLine()
			})
			if logs.Clicked() {
				go mygo.Shell.OpenPath(filepath.Join(a.dir, "logs"))
			}
		})
		ui.Text(c, hint+"  v"+version).FontSize(11.5).LineHeight(1.45).TextColor(t.TextMuted.Alpha(0.85)).Padding(0, 4)
	})
}

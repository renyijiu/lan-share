package main

import (
	"github.com/egoist/mygo/ui"
	"local/lan-share/internal/preferences"
	"local/lan-share/internal/smbserver"
	"testing"
)

func TestPasswordMaskAndRunningFields(t *testing.T) {
	a := &application{settings: preferences.Settings{Username: "projector", Folder: "/tmp/中文 媒体"}, password: "private-test-password", snapshot: smbserver.Snapshot{State: smbserver.Stopped}}
	tt := ui.NewTester(a.view, 740, 850)
	if err := tt.Click("密码"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Command("copy")
	if tt.Clipboard() != "" {
		t.Fatal("masked password was copied")
	}
	if err := tt.Click("显示"); err != nil {
		t.Fatal(err)
	}
	if !a.showPassword {
		t.Fatal("reveal password did not work")
	}
	if err := tt.Click("密码"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Command("copy")
	if tt.Clipboard() != a.password {
		t.Fatal("revealed password could not be selected and copied")
	}
	if err := tt.Click("隐藏"); err != nil {
		t.Fatal(err)
	}
	a.snapshot.State = smbserver.Running
	tt.Frame()
	if !tt.HasText("停止共享") {
		t.Fatal("running UI lacks Stop")
	}
	if err := tt.Click("用户名"); err != nil {
		t.Fatal(err)
	}
	tt.Type("changed")
	if a.settings.Username != "projector" {
		t.Fatal("running credentials were editable")
	}
}

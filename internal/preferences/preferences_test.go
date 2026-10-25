package preferences

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSettingsRoundTripAndReplace(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"第一次", "第二次"} {
		want := Settings{Folder: filepath.Join(dir, "中文 媒体"), Username: name, RememberPassword: true}
		if err := Save(dir, want); err != nil {
			t.Fatal(err)
		}
		got, err := Load(dir)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%+v %v", got, err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `"password"`) {
			t.Fatal("settings must not contain passwords")
		}
	}
}

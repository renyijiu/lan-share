package preferences

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Settings struct {
	Folder           string `json:"folder"`
	Username         string `json:"username"`
	RememberPassword bool   `json:"remember_password"`
}

// Passwords are intentionally absent from Settings and its JSON representation.
func Load(dir string) (Settings, error) {
	s := Settings{Username: "projector"}
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

func Save(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "settings.json"))
}

func Directory() (string, error) {
	if override := os.Getenv("LAN_SHARE_DATA_DIR"); override != "" {
		return filepath.Abs(override)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "LAN Share"), nil
}

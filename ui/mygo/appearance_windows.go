package desktop

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

func appearancePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goDisk", "appearance"), nil
}

func savedAppearance() mygo.ThemeSource {
	if path, err := appearancePath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			switch source := mygo.ThemeSource(strings.TrimSpace(string(data))); source {
			case mygo.ThemeDark, mygo.ThemeLight:
				return source
			}
		}
	}
	return mygo.ThemeSystem
}

func (d *desktop) setAppearance(dark bool) {
	d.syncAppearance(dark)
	source := mygo.ThemeLight
	if dark {
		source = mygo.ThemeDark
	}
	mygo.Theme.SetSource(source)
	if path, err := appearancePath(); err == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err == nil {
			_ = os.WriteFile(path, []byte(source), 0600)
		}
	}
}

func (d *desktop) syncAppearance(dark bool) {
	d.darkTheme = dark
	if d.themeMenu != nil {
		d.themeMenu.SetChecked(dark)
	}
}

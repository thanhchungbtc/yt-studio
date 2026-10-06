package desktop

import (
	"encoding/json"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Look is the window's appearance: theme ("system", "light", "dark") and
// material ("liquidGlass", "translucent", "solid").
type Look struct {
	Theme    string `json:"theme"`
	Material string `json:"material"`
}

// ReadLook reads the saved look, defaulting to the system theme and Liquid Glass.
func ReadLook(path string) Look {
	l := Look{Theme: "system", Material: "liquidGlass"}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &l)
	}
	return l
}

// SetLook applies the theme to the window now and saves both for the next
// launch (the material applies at launch).
func (p *Platform) SetLook(theme, material string) error {
	p.SetAppearance(theme)
	b, err := json.Marshal(Look{Theme: theme, Material: material})
	if err != nil {
		return err
	}
	return os.WriteFile(p.lookPath, b, 0o600)
}

// ApplyLook sets the window options for a saved look.
func ApplyLook(l Look, opts *application.WebviewWindowOptions) {
	opts.Mac.Appearance = WindowAppearance(l.Theme)
	switch l.Material {
	case "translucent":
		opts.Mac.Backdrop = application.MacBackdropTranslucent
	case "solid":
		opts.Mac.Backdrop = application.MacBackdropNormal
		opts.BackgroundType = application.BackgroundTypeSolid
	}
}

package desktop

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type windowState struct {
	application.Rect
	Maximised bool `json:"maximised"`
}

func readState(path string) (windowState, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return windowState{}, false
	}
	var ws windowState
	return ws, json.Unmarshal(raw, &ws) == nil
}

// ApplySavedBounds sets the window's initial bounds to where it was last.
func ApplySavedBounds(path string, opts *application.WebviewWindowOptions) {
	ws, ok := readState(path)
	if !ok || ws.Width < opts.MinWidth || ws.Height < opts.MinHeight {
		return
	}
	opts.Width, opts.Height = ws.Width, ws.Height
	opts.X, opts.Y = ws.X, ws.Y
	opts.InitialPosition = application.WindowXY
	if ws.Maximised {
		opts.StartState = application.WindowStateMaximised
	}
}

// RememberBounds saves the window's bounds to path after it moves or resizes.
func RememberBounds(path string, win *application.WebviewWindow) {
	var (
		mu    sync.Mutex
		timer *time.Timer
	)
	save := func(*application.WindowEvent) {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(400*time.Millisecond, func() {
			ws := windowState{Maximised: win.IsMaximised()}
			if !ws.Maximised {
				ws.Rect = win.Bounds()
			} else if prev, ok := readState(path); ok {
				// Keep the bounds it restores to.
				ws.Rect = prev.Rect
			}
			if ws.Width == 0 || ws.Height == 0 {
				return
			}
			if b, err := json.Marshal(ws); err == nil {
				_ = os.WriteFile(path, b, 0o600)
			}
		})
	}
	win.OnWindowEvent(events.Common.WindowDidResize, save)
	win.OnWindowEvent(events.Common.WindowDidMove, save)
}

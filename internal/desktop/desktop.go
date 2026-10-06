// Package desktop adapts the Wails runtime to the interfaces the app depends on.
package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Platform implements services.Emitter and services.Desktop.
type Platform struct {
	app      *application.App
	lookPath string
}

// New creates a Platform bound to app, saving the window look to lookPath.
func New(app *application.App, lookPath string) *Platform {
	return &Platform{app: app, lookPath: lookPath}
}

// Emit publishes an event to every window.
func (p *Platform) Emit(name string, data any) { p.app.Event.Emit(name, data) }

// OpenURL opens url in the default browser.
func (p *Platform) OpenURL(url string) error { return p.app.Browser.OpenURL(url) }

// RevealInFinder shows path in Finder.
func (p *Platform) RevealInFinder(path string) error { return p.app.Env.OpenFileManager(path, true) }

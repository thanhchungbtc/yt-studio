//go:build !(darwin && snapshot)

package desktop

import "github.com/wailsapp/wails/v3/pkg/application"

// MaybeRunSnapshots is a no-op unless built with -tags snapshot.
func MaybeRunSnapshots(*application.App, *application.WebviewWindow) {}

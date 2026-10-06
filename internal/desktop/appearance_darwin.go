//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// setAppAppearance sets NSApp.appearance: 1 = light, 2 = dark, 0 = follow
// the system. The native glass material follows it, so the window never
// shows a light material behind a dark UI (or vice versa).
static void setAppAppearance(int mode) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSAppearance *a = nil;
		if (mode == 1) a = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
		if (mode == 2) a = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
		[NSApp setAppearance:a];
	});
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// SetAppearance makes the native window material follow the app theme:
// "light", "dark" or "system".
func (p *Platform) SetAppearance(theme string) {
	C.setAppAppearance(appearanceMode(theme))
}

func appearanceMode(theme string) C.int {
	switch theme {
	case "light":
		return 1
	case "dark":
		return 2
	}
	return 0
}

// WindowAppearance maps a theme to the window option used at creation.
func WindowAppearance(theme string) application.MacAppearanceType {
	switch theme {
	case "light":
		return application.NSAppearanceNameAqua
	case "dark":
		return application.NSAppearanceNameDarkAqua
	}
	return application.DefaultAppearance
}

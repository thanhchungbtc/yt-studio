//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework Cocoa
int accessibilityFlags(void);
void observeAccessibility(void);
*/
import "C"

import "sync"

var (
	a11yMu        sync.Mutex
	a11yListeners []func(Accessibility)
	a11yOnce      sync.Once
)

// Accessibility returns the current macOS display accessibility settings.
func (p *Platform) Accessibility() Accessibility {
	return readAccessibility()
}

func readAccessibility() Accessibility {
	f := int(C.accessibilityFlags())
	return Accessibility{ReduceTransparency: f&1 != 0, ReduceMotion: f&2 != 0, IncreaseContrast: f&4 != 0}
}

// OnAccessibilityChange calls fn with the new settings whenever the user
// changes them in System Settings.
func (p *Platform) OnAccessibilityChange(fn func(Accessibility)) {
	a11yMu.Lock()
	a11yListeners = append(a11yListeners, fn)
	a11yMu.Unlock()
	a11yOnce.Do(func() { C.observeAccessibility() })
}

//export goAccessibilityChanged
func goAccessibilityChanged() {
	a := readAccessibility()
	a11yMu.Lock()
	ls := append([]func(Accessibility){}, a11yListeners...)
	a11yMu.Unlock()
	for _, fn := range ls {
		go fn(a)
	}
}

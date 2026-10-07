package services

import (
	"net/url"

	"github.com/tbui/yt-studio/internal/app"
)

// Desktop is what the system service needs from the OS.
type Desktop interface {
	OpenURL(url string) error
	RevealInFinder(path string) error
	SetLook(theme, material string) error
}

// AccessibilityEvent is emitted when macOS display accessibility changes.
const AccessibilityEvent = "system:accessibility"

// AccessibilityOptions are the macOS display settings the UI follows.
type AccessibilityOptions struct {
	ReduceTransparency bool `json:"reduceTransparency"`
	ReduceMotion       bool `json:"reduceMotion"`
	IncreaseContrast   bool `json:"increaseContrast"`
}

// SystemService is the app's bridge to the system.
type SystemService struct {
	desktop       Desktop
	version       string
	home          string
	accessibility func() AccessibilityOptions
}

// NewSystemService creates a SystemService.
func NewSystemService(desktop Desktop, version, home string, accessibility func() AccessibilityOptions) *SystemService {
	return &SystemService{desktop: desktop, version: version, home: home, accessibility: accessibility}
}

// Accessibility returns the macOS display accessibility settings.
func (s *SystemService) Accessibility() AccessibilityOptions { return s.accessibility() }

// Info describes the running app.
type Info struct {
	Version string `json:"version"`
	Home    string `json:"home"`
}

// Info returns the running version and where its data lives.
func (s *SystemService) Info() Info { return Info{Version: s.version, Home: s.home} }

// OpenURL opens a web link in the default browser.
func (s *SystemService) OpenURL(link string) error {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return app.Invalid("url", "must be a web link")
	}
	return s.desktop.OpenURL(u.String())
}

// SetLook applies the theme to the window now; the material applies at the
// next launch.
func (s *SystemService) SetLook(theme, material string) error {
	return s.desktop.SetLook(theme, material)
}

// Reveal shows a file or folder in Finder.
func (s *SystemService) Reveal(path string) error { return s.desktop.RevealInFinder(path) }

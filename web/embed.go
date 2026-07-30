// Package web carries the built React UI into the binary.
//
// `go generate ./web` builds the frontend and stages it here; the production
// binary then contains the assets and serves them from /. One static file ships
// the whole application — no separate web server, no separate deploy.
package web

import (
	"embed"
	"io/fs"
)

//go:generate npm --prefix . ci
//go:generate npm --prefix . run build

//go:embed all:dist
var distFS embed.FS

// Dist returns the built UI rooted at its index.html.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

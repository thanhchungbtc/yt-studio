package http

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// bundleDir is where Vite writes its content-hashed output. Not "assets": that
// prefix is the server's artifact route.
const bundleDir = "app"

// spaHandler serves the built React app from an embed.FS. An unknown path falls
// through to index.html, because the client owns routing.
func spaHandler(dist fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if dist == nil {
			http.Error(w, "web UI is not embedded in this build", http.StatusNotFound)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}

		f, err := dist.Open(name)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "failed to read web asset", http.StatusInternalServerError)
				return
			}
			serveIndex(w, r, dist)
			return
		}
		defer func() { _ = f.Close() }()

		info, err := f.Stat()
		if err != nil || info.IsDir() {
			serveIndex(w, r, dist)
			return
		}
		// Names under /app are content-hashed and so immutable; a cached
		// index.html would point at bundles a rebuild has already replaced.
		if strings.HasPrefix(name, bundleDir+"/") {
			w.Header().Set("Cache-Control", immutableCacheControl)
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		serveFile(w, r, name, f)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	f, err := dist.Open("index.html")
	if err != nil {
		http.Error(w, "web UI is not embedded in this build", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Cache-Control", "no-cache")
	serveFile(w, r, "index.html", f)
}

func serveFile(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	// embed.FS always yields a ReadSeeker; this is the defensive path.
	w.Header().Set("Content-Type", contentTypeFor(name))
	_, _ = io.Copy(w, f)
}

func contentTypeFor(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

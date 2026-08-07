package http

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
)

// resourceMIME is the whole set of operator-supplied files this route will
// serve, by extension.
//
// A whitelist rather than a file server over the directory: `var/resources`
// also holds bg.mp4, which is over a gigabyte, and nothing in the browser wants
// it. What is here is what the thumbnail editor has to draw with — the
// typefaces the Go renderer parses, and the backdrop it composes onto.
var resourceMIME = map[string]string{
	".ttf":  "font/ttf",
	".otf":  "font/otf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
}

// resourceHandler serves one file from the resources directory, so the browser
// thumbnail editor can compose against the same background and the same
// typefaces the builtin renderer uses. Without it the editor would lay out text
// in a substitute face over a placeholder, and the operator would be designing
// something other than what they get.
//
// A file route rather than an API operation, for the same reason /assets is:
// what comes back is a font or a photograph, and @font-face and <img> want a
// URL, not a JSON envelope.
//
// A nil directory is a server started without resources: 404, not a panic.
func resourceHandler(resources fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if resources == nil {
			http.Error(w, "no resources directory", http.StatusNotFound)
			return
		}
		name, mime, ok := resolveResource(chi.URLParam(r, "*"))
		if !ok {
			http.Error(w, "not a servable resource", http.StatusNotFound)
			return
		}
		f, err := resources.Open(name)
		if err != nil {
			http.Error(w, "resource not found", http.StatusNotFound)
			return
		}
		defer func() { _ = f.Close() }()

		w.Header().Set("Content-Type", mime)
		// Revalidated rather than immutable: unlike /assets the name is not a
		// content address, so replacing a typeface or the backdrop on disk has
		// to reach the editor.
		w.Header().Set("Cache-Control", "no-cache")
		if _, err := io.Copy(w, f); err != nil {
			// The response is already committed; the client went away.
			return
		}
	}
}

// resolveResource turns a request path into a name inside the resources
// directory, or reports that it is not one this route serves.
//
// fs.FS rejects a path that escapes its root on its own, but the check is
// explicit here so the reachable set is legible: no traversal, no absolute
// path, and one of the extensions above.
func resolveResource(raw string) (name, mime string, ok bool) {
	name = path.Clean(strings.TrimPrefix(raw, "/"))
	if name == "." || name == "/" || strings.HasPrefix(name, "..") || path.IsAbs(name) {
		return "", "", false
	}
	mime, ok = resourceMIME[strings.ToLower(path.Ext(name))]
	if !ok {
		return "", "", false
	}
	return name, mime, true
}

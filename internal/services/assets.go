package services

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/provider"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

// resourceMIME is the kinds of resource the UI loads.
var resourceMIME = map[string]string{
	".ttf":  "font/ttf",
	".otf":  "font/otf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
}

// AssetMiddleware serves /assets/{id} and /resources/{path}; the rest goes to next.
func AssetMiddleware(assets repository.AssetReader, store provider.AssetStore, resources fs.FS) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/assets/"):
				serveAsset(w, r, assets, store, strings.TrimPrefix(r.URL.Path, "/assets/"))
			case strings.HasPrefix(r.URL.Path, "/resources/"):
				serveResource(w, r, resources, strings.TrimPrefix(r.URL.Path, "/resources/"))
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

func serveAsset(w http.ResponseWriter, r *http.Request, assets repository.AssetReader, store provider.AssetStore, id string) {
	opened, err := app.OpenAsset(r.Context(), assets, store, entity.AssetID(id))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, entity.ErrAssetNotFound) {
			http.Error(w, "asset not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to read asset", http.StatusInternalServerError)
		return
	}
	defer func() { _ = opened.Reader.Close() }()
	h := w.Header()
	h.Set("Content-Type", opened.Asset.MIME)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("ETag", `"`+string(opened.Asset.ID)+`"`)
	h.Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, string(opened.Asset.ID)+opened.Asset.Kind.Ext(), opened.Asset.CreatedAt, opened.Reader)
}

func serveResource(w http.ResponseWriter, r *http.Request, resources fs.FS, raw string) {
	name := path.Clean(raw)
	mime, ok := resourceMIME[strings.ToLower(path.Ext(name))]
	if !ok || !fs.ValidPath(name) || name == "." {
		http.Error(w, "not a servable resource", http.StatusNotFound)
		return
	}
	b, err := fs.ReadFile(resources, name)
	if err != nil {
		http.Error(w, "resource not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", mime)
	// The operator can replace a resource at any time.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

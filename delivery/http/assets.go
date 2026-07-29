package http

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// immutableCacheControl is safe because the URL contains the content address:
// the hash is the cache key, so a still caches forever for free (§9).
const immutableCacheControl = "public, max-age=31536000, immutable"

// thumbnailWidth is what the chapter grid requests instead of the full still.
const thumbnailWidth = 160

// assetHandler streams a stored artifact.
//
// http.ServeContent is used rather than io.Copy so range requests work: the
// operator can scrub a three-hour render without the server buffering it.
func assetHandler(assets repository.AssetReader, store provider.AssetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := entity.AssetID(chi.URLParam(r, "id"))
		opened, err := app.OpenAsset(r.Context(), assets, store, id)
		if err != nil {
			writeAssetError(w, err)
			return
		}
		defer func() { _ = opened.Reader.Close() }()

		w.Header().Set("Content-Type", opened.Asset.MIME)
		w.Header().Set("Cache-Control", immutableCacheControl)
		w.Header().Set("ETag", `"`+string(opened.Asset.ID)+`"`)
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, string(opened.Asset.ID)+opened.Asset.Kind.Ext(), opened.Asset.CreatedAt, opened.Reader)
	}
}

// thumbnailHandler serves a downscaled still for grid views. Results are cached
// in memory keyed by content address, so the decode happens once per image for
// the lifetime of the process.
func thumbnailHandler(assets repository.AssetReader, store provider.AssetStore, cache *thumbnailCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := entity.AssetID(chi.URLParam(r, "id"))
		if cached, ok := cache.get(id); ok {
			serveThumbnail(w, r, id, cached)
			return
		}
		encoded, err := renderThumbnail(r.Context(), assets, store, id)
		if err != nil {
			writeAssetError(w, err)
			return
		}
		cache.put(id, encoded)
		serveThumbnail(w, r, id, encoded)
	}
}

func serveThumbnail(w http.ResponseWriter, r *http.Request, id entity.AssetID, body []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", immutableCacheControl)
	w.Header().Set("ETag", `"`+string(id)+`-thumb"`)
	http.ServeContent(w, r, string(id)+".png", zeroTime, bytes.NewReader(body))
}

func renderThumbnail(ctx context.Context, assets repository.AssetReader, store provider.AssetStore, id entity.AssetID) ([]byte, error) {
	opened, err := app.OpenAsset(ctx, assets, store, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = opened.Reader.Close() }()

	src, err := png.Decode(opened.Reader)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, downscale(src, thumbnailWidth)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// downscale is a box filter: each destination pixel averages the source pixels
// it covers. It needs no dependency and is exact enough for a grid thumbnail.
func downscale(src image.Image, width int) image.Image {
	b := src.Bounds()
	if b.Dx() <= width {
		return src
	}
	height := b.Dy() * width / b.Dx()
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xStep := float64(b.Dx()) / float64(width)
	yStep := float64(b.Dy()) / float64(height)

	for y := range height {
		for x := range width {
			x0 := b.Min.X + int(float64(x)*xStep)
			x1 := b.Min.X + int(float64(x+1)*xStep)
			y0 := b.Min.Y + int(float64(y)*yStep)
			y1 := b.Min.Y + int(float64(y+1)*yStep)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			var rs, gs, bs, as, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					rs += uint64(cr)
					gs += uint64(cg)
					bs += uint64(cb)
					as += uint64(ca)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.Set(x, y, colorFrom(rs/n, gs/n, bs/n, as/n))
		}
	}
	return dst
}

func writeAssetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, entity.ErrAssetNotFound):
		http.Error(w, "asset not found", http.StatusNotFound)
	default:
		http.Error(w, "failed to read asset", http.StatusInternalServerError)
	}
}

// thumbnailCache is a bounded map of rendered thumbnails. Eviction is a full
// clear at the cap: thumbnails are tens of kilobytes and rebuilding one is a
// single PNG decode, so an LRU would cost more than it saves.
type thumbnailCache struct {
	mu      sync.RWMutex
	entries map[entity.AssetID][]byte
	cap     int
}

func newThumbnailCache(capacity int) *thumbnailCache {
	if capacity <= 0 {
		capacity = 512
	}
	return &thumbnailCache{entries: make(map[entity.AssetID][]byte, capacity), cap: capacity}
}

func (c *thumbnailCache) get(id entity.AssetID) ([]byte, bool) {
	c.mu.RLock()
	b, ok := c.entries[id]
	c.mu.RUnlock()
	return b, ok
}

func (c *thumbnailCache) put(id entity.AssetID, body []byte) {
	c.mu.Lock()
	if len(c.entries) >= c.cap {
		clear(c.entries)
	}
	c.entries[id] = body
	c.mu.Unlock()
}

// zeroTime disables ServeContent's modification-time handling for bodies whose
// identity is already the content address.
var zeroTime = time.Time{}

func colorFrom(r, g, b, a uint64) color.RGBA {
	return color.RGBA{
		R: uint8(r >> 8), //nolint:gosec // RGBA() returns 16-bit channels
		G: uint8(g >> 8), //nolint:gosec // as above
		B: uint8(b >> 8), //nolint:gosec // as above
		A: uint8(a >> 8), //nolint:gosec // as above
	}
}

// AssetsOutput lists a video's artifacts.
type AssetsOutput struct {
	Body struct {
		Assets []AssetDTO `json:"assets"`
	}
}

func getVideoAssets(videos repository.VideoReader, assets repository.AssetReader) func(context.Context, *VideoKeyInput) (*AssetsOutput, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*AssetsOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		rows, err := app.ListAssets(ctx, assets, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &AssetsOutput{}
		out.Body.Assets = make([]AssetDTO, 0, len(rows))
		for _, a := range rows {
			out.Body.Assets = append(out.Body.Assets, assetFrom(a))
		}
		return out, nil
	}
}

func registerAssetRoutes(api huma.API, videos repository.VideoReader, assets repository.AssetReader) {
	huma.Register(api, huma.Operation{
		OperationID: "listVideoAssets", Method: "GET", Path: "/api/videos/{key}/assets",
		Summary: "List a video's artifacts", Tags: []string{"assets"},
	}, getVideoAssets(videos, assets))
}

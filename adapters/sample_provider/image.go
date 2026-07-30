package sampleprovider

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"sync"

	// Registered for its decoder: the samples are JPEG on disk and PNG in the
	// store.
	_ "image/jpeg"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Image serves stills from the sample set, rotated across chapters.
type Image struct {
	lib   *Library
	store provider.AssetStore

	// encoded caches the PNG bytes per source file. A video asks for a hundred
	// stills and there are only a handful of files behind them, so the decode and
	// re-encode happen once each rather than once per chapter.
	mu      sync.Mutex
	encoded map[string][]byte
}

var _ provider.ImageProvider = (*Image)(nil)

// NewImage wires the backend to the shared library.
func NewImage(lib *Library, store provider.AssetStore) *Image {
	return &Image{lib: lib, store: store, encoded: make(map[string][]byte, 4)}
}

// Generate stores one still and returns its content address.
//
// The file is chosen by ordinal and index together, so a chapter always gets
// distinct stills — a dissolve between two copies of one image is not a
// dissolve — and consecutive chapters start at different points in the set
// rather than repeating the same pair down the whole video.
func (i *Image) Generate(ctx context.Context, req provider.ImageRequest) (entity.AssetID, error) {
	if err := i.lib.Check(); err != nil {
		return "", err
	}
	path := i.lib.images[(req.Ordinal+req.Index)%len(i.lib.images)]

	encoded, err := i.pngFor(path)
	if err != nil {
		return "", err
	}
	stored, err := i.store.Put(ctx, entity.AssetKindImage, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("store still: %w", err)
	}
	return stored.ID, nil
}

// pngFor decodes a sample and re-encodes it as PNG.
//
// The stills are JPEG on disk but an image asset is a .png with an image/png
// MIME, and the store addresses content rather than sniffing it. Normalising
// here keeps every consumer downstream — the browser, the composer, anything
// added later — able to trust what the asset row says it is.
func (i *Image) pngFor(path string) ([]byte, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if cached, ok := i.encoded[path]; ok {
		return cached, nil
	}

	file, err := os.Open(path) //nolint:gosec // path comes from the resources directory
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrUnavailable, path, err)
	}
	defer func() { _ = file.Close() }()

	src, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrUnavailable, path, err)
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, src); err != nil {
		return nil, fmt.Errorf("encode still %s: %w", path, err)
	}

	i.encoded[path] = buf.Bytes()
	return buf.Bytes(), nil
}

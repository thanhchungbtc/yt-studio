package sampleprovider

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"sync"

	// Registered for its decoder: the samples are JPEG on disk and PNG in the
	// store.
	_ "image/jpeg"
)

// pngCache holds the encoded bytes of every sample a backend has served.
//
// The samples are JPEG on disk but every image asset is a .png with an
// image/png MIME, and the store addresses content rather than sniffing it.
// Normalising on the way in keeps every consumer downstream — the browser, the
// composer, the thumbnail renderer — able to trust what the asset row says.
//
// A video asks for a hundred stills and ten icons from a handful of files, so
// each conversion happens once and is then handed out by content address.
type pngCache struct {
	mu      sync.Mutex
	entries map[string][]byte
}

// bytes returns the PNG for a key, converting through prepare on first ask.
// prepare is given the decoded source and returns what should be encoded, which
// is what lets the icon backend crop and the still backend pass through.
func (c *pngCache) bytes(key, path string, prepare func(image.Image) image.Image) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.entries[key]; ok {
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
	if prepare != nil {
		src = prepare(src)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, src); err != nil {
		return nil, fmt.Errorf("encode %s: %w", path, err)
	}

	if c.entries == nil {
		c.entries = make(map[string][]byte, 8)
	}
	c.entries[key] = buf.Bytes()
	return buf.Bytes(), nil
}

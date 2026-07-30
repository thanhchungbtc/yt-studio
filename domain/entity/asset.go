package entity

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidAsset is returned by the Asset constructor for invalid input.
var ErrInvalidAsset = errors.New("invalid asset")

// AssetKind classifies a generated file.
type AssetKind string

// The complete set of asset kinds.
const (
	AssetKindBlueprint AssetKind = "blueprint"
	AssetKindScript    AssetKind = "script"
	AssetKindPrompt    AssetKind = "prompt"
	AssetKindAudio     AssetKind = "audio"
	AssetKindImage     AssetKind = "image"
	AssetKindClip      AssetKind = "clip"
	AssetKindFinal     AssetKind = "final"
	AssetKindMetadata  AssetKind = "metadata"
	AssetKindThumbnail AssetKind = "thumbnail"
)

// AllAssetKinds lists every AssetKind, for validation and the UI.
var AllAssetKinds = []AssetKind{
	AssetKindBlueprint,
	AssetKindScript,
	AssetKindPrompt,
	AssetKindAudio,
	AssetKindImage,
	AssetKindClip,
	AssetKindFinal,
	AssetKindMetadata,
	AssetKindThumbnail,
}

// Valid reports whether the kind is one of the known constants.
func (k AssetKind) Valid() bool {
	switch k {
	case AssetKindBlueprint, AssetKindScript, AssetKindPrompt, AssetKindAudio,
		AssetKindImage, AssetKindClip, AssetKindFinal, AssetKindMetadata, AssetKindThumbnail:
		return true
	default:
		return false
	}
}

// Ext returns the canonical file extension for the kind, including the dot.
func (k AssetKind) Ext() string {
	switch k {
	case AssetKindBlueprint, AssetKindMetadata:
		return ".json"
	case AssetKindScript, AssetKindPrompt:
		return ".txt"
	case AssetKindAudio:
		return ".wav"
	case AssetKindImage, AssetKindThumbnail:
		return ".png"
	case AssetKindClip, AssetKindFinal:
		return ".mp4"
	default:
		return ".bin"
	}
}

// MIME returns the content type served for the kind.
func (k AssetKind) MIME() string {
	switch k {
	case AssetKindBlueprint, AssetKindMetadata:
		return "application/json"
	case AssetKindScript, AssetKindPrompt:
		return "text/plain; charset=utf-8"
	case AssetKindAudio:
		return "audio/wav"
	case AssetKindImage, AssetKindThumbnail:
		return "image/png"
	case AssetKindClip, AssetKindFinal:
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

// Asset is a content-addressed file on disk plus its metadata. The ID is the
// sha256 of the bytes, so identical output re-uses the same row and the same
// file — this is what makes a partial re-run cheap.
type Asset struct {
	ID        AssetID
	VideoID   VideoID
	ChapterID *ChapterID
	Kind      AssetKind
	// Path is relative to the configured asset root, never absolute, so the store
	// can be moved without rewriting rows.
	Path string
	Size int64
	MIME string
	// Provenance records which provider and task produced the file.
	Provenance string
	CreatedAt  time.Time
}

// NewAsset validates and constructs an Asset.
func NewAsset(id AssetID, videoID VideoID, chapterID *ChapterID, kind AssetKind, path string, size int64, provenance string, now time.Time) (Asset, error) {
	if len(id) != 64 {
		return Asset{}, fmt.Errorf("%w: id must be a 64-character sha256 hex digest, got %d characters", ErrInvalidAsset, len(id))
	}
	if !kind.Valid() {
		return Asset{}, fmt.Errorf("%w: unknown kind %q", ErrInvalidAsset, kind)
	}
	if strings.TrimSpace(path) == "" {
		return Asset{}, fmt.Errorf("%w: path must not be empty", ErrInvalidAsset)
	}
	if size < 0 {
		return Asset{}, fmt.Errorf("%w: size must not be negative", ErrInvalidAsset)
	}
	return Asset{
		ID:         id,
		VideoID:    videoID,
		ChapterID:  chapterID,
		Kind:       kind,
		Path:       path,
		Size:       size,
		MIME:       kind.MIME(),
		Provenance: provenance,
		CreatedAt:  now,
	}, nil
}

// ErrAssetNotFound is returned by asset lookups when the content address is
// unknown to the store.
var ErrAssetNotFound = errors.New("asset not found")

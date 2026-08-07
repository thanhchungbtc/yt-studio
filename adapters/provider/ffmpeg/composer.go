// Package ffmpeg is the real composition backend: chapter clips and the final
// render, produced by invoking ffmpeg with explicit argv.
//
// No wrapper library. Every invocation builds a []string and logs it, so a
// failed composition can be pasted straight into a shell. A request carries
// every string that reaches the output, titles included, so nothing here reads
// a database, a setting or a clock.
package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a missing binary or resource file. Wrapping the port's
// sentinel stops a task retrying for a binary that will not appear.
var ErrUnavailable = fmt.Errorf("ffmpeg composer: %w", provider.ErrUnavailable)

// Chapter clip geometry and timing. These are the reference pipeline's
// constants and the output is defined by them; they are not tunable.
const (
	imageWidth       = 1344
	imageHeight      = 768
	videoFPS         = 30
	titleStripHeight = 130
	titleFontMax     = 48
	titleFontMin     = 20
	titleFontStep    = 2
	// videoTitleSize is the second, fixed-size line under the chapter title.
	videoTitleSize = 22
	// titleMaxWidth is the width the chapter title is shrunk to fit.
	titleMaxWidth = imageWidth - 40

	slideCrossfade   = 0.5
	chapterCrossfade = 1.0
	chapterTailPad   = 1.5
)

// Final render geometry.
const (
	concatWidth       = 1920
	concatHeight      = 1080
	concatImageWidth  = 1344
	concatImageHeight = 768
)

// Filter-graph literals. The numbers above are formatted exactly once, here,
// because a filter graph is a string and "1.0" is not "1".
const (
	slideCrossfadeArg = "0.5"
	crossfadeArg      = "1.0"
	// headPadArg and tailPadArg pad a chapter for the crossfade into its
	// neighbours, plus the trailing beat at the end.
	headPadArg    = "1.0"
	tailPadArg    = "2.5" // chapterCrossfade + chapterTailPad
	adelayMillis  = "1000"
	bgMusicVolume = "0.35"
	audioBitrate  = "192k"
)

// LocalStore is the asset store seen from here: ffmpeg takes filenames rather
// than readers, so this consumer needs two things beyond the port — where an
// asset lives, and how to hand a finished file back without copying it.
type LocalStore interface {
	provider.AssetStore
	Root() string
	Path(id entity.AssetID, kind entity.AssetKind) (string, error)
	PutFile(ctx context.Context, kind entity.AssetKind, src string) (provider.StoredAsset, error)
}

// Resources are the fixed production assets every render is built on.
type Resources struct {
	Dir        string
	Chalkboard string
	BgVideo    string
	BgMusic    string
	TitleFont  string
}

// NewResources locates the resource files inside a directory.
func NewResources(dir string) Resources {
	return Resources{
		Dir:        dir,
		Chalkboard: filepath.Join(dir, "chalkboard.jpg"),
		BgVideo:    filepath.Join(dir, "bg.mp4"),
		BgMusic:    filepath.Join(dir, "bg.mp3"),
		TitleFont:  filepath.Join(dir, "fonts", "CabinSketch-Bold.ttf"),
	}
}

// Composer implements provider.VideoComposer against the ffmpeg binary.
type Composer struct {
	store LocalStore
	res   Resources
	log   *slog.Logger
	// work is the scratch root, kept under the store root so every intermediate
	// file lands in var/ and the finished render can be renamed into place rather
	// than copied.
	work string
	// lanes bounds the slide encodes within one chapter. The compose pool already
	// bounds chapters against each other; this keeps a single chapter from taking
	// the whole machine.
	lanes int

	preflightOnce sync.Once
	preflightErr  error
	fonts         fontCache
}

var _ provider.VideoComposer = (*Composer)(nil)

// New wires the composer. It does not touch the filesystem: a missing binary or
// resource surfaces on the first composition, or from Check.
func New(store LocalStore, resourcesDir string, log *slog.Logger) *Composer {
	if log == nil {
		log = slog.Default()
	}
	lanes := runtime.NumCPU() / 2
	if lanes < 1 {
		lanes = 1
	}
	if lanes > 4 {
		lanes = 4
	}
	return &Composer{
		store: store,
		res:   NewResources(resourcesDir),
		log:   log,
		work:  filepath.Join(store.Root(), ".tmp"),
		lanes: lanes,
	}
}

// Check reports whether this backend can run, so startup can say so once rather
// than failing the first chapter of a fifty-chapter video.
func (c *Composer) Check() error {
	c.preflightOnce.Do(func() { c.preflightErr = c.preflight() })
	return c.preflightErr
}

func (c *Composer) preflight() error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%w: %s is not on PATH", ErrUnavailable, bin)
		}
	}
	// bg.mp3 and the font are optional — the reference pipeline degrades to no
	// music and to ffmpeg's built-in font. The chalkboard and the background video
	// are not: without them the output is a different video.
	for _, f := range []string{c.res.Chalkboard, c.res.BgVideo} {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrUnavailable, f, err)
		}
	}
	return nil
}

// tempDir creates a scratch directory under the store root.
func (c *Composer) tempDir(prefix string) (string, func(), error) {
	if err := os.MkdirAll(c.work, 0o755); err != nil {
		return "", nil, fmt.Errorf("create work dir: %w", err)
	}
	dir, err := os.MkdirTemp(c.work, prefix+"-*")
	if err != nil {
		return "", nil, fmt.Errorf("create scratch dir: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// inputPaths resolves asset ids to on-disk paths.
func (c *Composer) inputPaths(ids []entity.AssetID, kind entity.AssetKind) ([]string, error) {
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		p, err := c.store.Path(id, kind)
		if err != nil {
			return nil, fmt.Errorf("resolve %s %s: %w", kind, id.Short(), err)
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("%s %s: %w", kind, id.Short(), entity.ErrAssetNotFound)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

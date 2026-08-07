package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// ErrSweepUnsafe is what a sweep pointed at the wrong database looks like from
// the inside: a store full of assets the database references none of. Nothing
// here can tell that apart from a store that is genuinely all garbage.
var ErrSweepUnsafe = errors.New("refusing to sweep: the database references no assets at all")

// defaultTempAge is how old a staged file must be before it counts as the
// debris of a crashed write rather than a write in progress.
const defaultTempAge = time.Hour

// unrecognisedSample bounds the sample of surprising paths; the count stays
// exact.
const unrecognisedSample = 20

// SweepOptions tunes one sweep.
type SweepOptions struct {
	// Apply makes the sweep delete. Left false it only reports, which is the
	// CLI's default because the deletion is irreversible.
	Apply bool
	// TempAge overrides the staging-area cutoff; zero means defaultTempAge.
	TempAge time.Duration
	// Now is the injected clock.
	Now time.Time
	// Force skips the empty-database guard.
	Force bool
}

// SweepReport is what a sweep found.
type SweepReport struct {
	// Files is every regular file under the store root.
	Files int
	// Referenced files have an owner in the database and are left alone.
	Referenced int
	// Unreferenced files are content addresses nothing owns.
	Unreferenced int
	// Debris is old staging-area files, left by writes that crashed between
	// streaming the bytes and renaming them into place.
	Debris int
	// Unrecognised counts files the layout does not explain. They are never
	// deleted: nothing in the database describes them, so nothing licenses it.
	Unrecognised int
	// UnrecognisedSample lists the first few, so they can be looked at.
	UnrecognisedSample []string
	// Removed and Bytes are zero unless Apply was set.
	Removed int
	Bytes   int64
	// DirsRemoved counts the empty directories pruned afterwards. A dry run
	// leaves it zero rather than guessing which would have ended up empty.
	DirsRemoved int
}

// Reclaimable is what a dry run would free.
func (r SweepReport) Reclaimable() int { return r.Unreferenced + r.Debris }

// SweepAssets reclaims files nothing references. It walks the store rather than
// reading the database, because a file whose row was lost is exactly what needs
// finding. An unowned content address goes, an old staged file goes, and
// anything the layout does not explain is reported and kept.
//
// The caller must have run RepairAssetOwnership first: without it, a file
// reachable only through a chapter's id list looks like garbage.
func SweepAssets(
	ctx context.Context,
	maintainer repository.AssetMaintainer,
	sweeper provider.AssetSweeper,
	opts SweepOptions,
	log *slog.Logger,
) (SweepReport, error) {
	addresses, err := maintainer.AssetAddresses(ctx)
	if err != nil {
		return SweepReport{}, err
	}
	live := make(map[entity.AssetID]struct{}, len(addresses))
	for _, a := range addresses {
		live[a.ID] = struct{}{}
	}

	tempAge := opts.TempAge
	if tempAge <= 0 {
		tempAge = defaultTempAge
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	// Classify everything before deleting anything: the guard below needs the
	// whole count, which a single pass would already have destroyed.
	var report SweepReport
	var doomed []provider.StoredFile
	err = sweeper.Walk(ctx, func(f provider.StoredFile) error {
		report.Files++
		switch {
		case f.Temporary:
			// A young temporary file may be a write happening now.
			if now.Sub(f.ModTime) < tempAge {
				return nil
			}
			report.Debris++
			doomed = append(doomed, f)
		case f.ID == "":
			report.Unrecognised++
			if len(report.UnrecognisedSample) < unrecognisedSample {
				report.UnrecognisedSample = append(report.UnrecognisedSample, f.Rel)
			}
		default:
			if _, ok := live[f.ID]; ok {
				report.Referenced++
				return nil
			}
			report.Unreferenced++
			doomed = append(doomed, f)
		}
		return nil
	})
	if err != nil {
		return report, fmt.Errorf("walk the asset store: %w", err)
	}

	// The guard is on the deletion, not the counting: a report changes nothing
	// and is the fastest way to see the database does not go with this store.
	if opts.Apply && !opts.Force && len(live) == 0 && report.Referenced+report.Unreferenced > 0 {
		return report, fmt.Errorf("%w, but the store holds %d (wrong --db, or a database that was never migrated?)",
			ErrSweepUnsafe, report.Unreferenced)
	}

	if opts.Apply {
		for _, f := range doomed {
			if err := sweeper.Remove(ctx, f.Rel); err != nil {
				return report, err
			}
			report.Removed++
			report.Bytes += f.Size
		}
		// A shard is shared by every address with the same digest prefix, so
		// emptiness is only knowable once the files are gone.
		pruned, err := sweeper.PruneEmptyDirs(ctx)
		if err != nil {
			return report, err
		}
		report.DirsRemoved = pruned
	}

	log.Info("swept the asset store",
		slog.Bool("applied", opts.Apply),
		slog.Int("files", report.Files),
		slog.Int("referenced", report.Referenced),
		slog.Int("unreferenced", report.Unreferenced),
		slog.Int("debris", report.Debris),
		slog.Int("unrecognised", report.Unrecognised),
		slog.Int("removed", report.Removed),
		slog.Int64("bytes", report.Bytes),
		slog.Int("dirs_removed", report.DirsRemoved))
	return report, nil
}

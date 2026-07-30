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

// ErrSweepUnsafe is returned when the store holds assets and the database
// references none of them. It is what a sweep pointed at the wrong database
// looks like from the inside, and the difference between that and a store that
// is genuinely all garbage is not something this command can tell.
var ErrSweepUnsafe = errors.New("refusing to sweep: the database references no assets at all")

// defaultTempAge is how old a file in the staging area must be before the sweep
// treats it as the debris of a crashed write rather than a write in progress.
const defaultTempAge = time.Hour

// unrecognisedSample bounds how many surprising paths a report carries. The
// count is always exact; the list is there to give a person somewhere to look.
const unrecognisedSample = 20

// SweepOptions tunes one sweep.
type SweepOptions struct {
	// Apply makes the sweep delete. Left false it only reports, which is the
	// default the CLI uses: the numbers are the interesting part and the deletion
	// is irreversible.
	Apply bool
	// TempAge overrides how old debris in the staging area must be. Zero means
	// defaultTempAge.
	TempAge time.Duration
	// Now is the clock, injected so a test does not have to sleep.
	Now time.Time
	// Force skips the empty-database guard, for the rare store that really does
	// contain nothing but garbage.
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
	// Debris is old files in the staging area, left by writes that crashed
	// between streaming the bytes and renaming them into place.
	Debris int
	// Unrecognised counts files the store layout does not explain. They are never
	// deleted: nothing in the database describes them, so nothing in the database
	// can license removing them.
	Unrecognised int
	// UnrecognisedSample lists the first few of those, so they can be looked at.
	UnrecognisedSample []string
	// Removed and Bytes report what was actually unlinked, which is zero unless
	// Apply was set.
	Removed int
	Bytes   int64
}

// Reclaimable is what a dry run would free.
func (r SweepReport) Reclaimable() int { return r.Unreferenced + r.Debris }

// SweepAssets reclaims files in the store that nothing references.
//
// The store is walked rather than the database read, because a file whose row
// was lost is exactly what needs finding and it is invisible from the database
// side. Three kinds of file come out of that walk and each is treated
// differently: a content address with no owner is garbage and goes; an old file
// in the staging area is a crashed write and goes; anything else the layout does
// not explain is reported and kept, because no database lookup describes it and
// so none can justify deleting it.
//
// The caller must have run RepairAssetOwnership first. Without it, a file that a
// surviving video reaches only through its chapter id lists has no owning row,
// and this command cannot tell that from garbage.
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

	// Classify everything first and delete nothing: the whole store has to be
	// counted before the guard below can tell a store full of garbage from a
	// sweep pointed at the wrong database, and by then a single-pass version
	// would already have deleted the evidence.
	var report SweepReport
	var doomed []provider.StoredFile
	err = sweeper.Walk(ctx, func(f provider.StoredFile) error {
		report.Files++
		switch {
		case f.Temporary:
			// A young temporary file may be a write happening right now.
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

	// The guard is on the deletion, not on the counting: a report changes nothing,
	// and it is the fastest way to see that the database is not the one that goes
	// with this store.
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
	}

	log.Info("swept the asset store",
		slog.Bool("applied", opts.Apply),
		slog.Int("files", report.Files),
		slog.Int("referenced", report.Referenced),
		slog.Int("unreferenced", report.Unreferenced),
		slog.Int("debris", report.Debris),
		slog.Int("unrecognised", report.Unrecognised),
		slog.Int("removed", report.Removed),
		slog.Int64("bytes", report.Bytes))
	return report, nil
}

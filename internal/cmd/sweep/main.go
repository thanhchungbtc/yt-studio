// Command sweep reclaims asset files nothing in the database references
// (`make sweep`). It only reports unless -apply is given. Quit the app first.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/tbui/yt-studio/internal/adapters/assetstore"
	"github.com/tbui/yt-studio/internal/adapters/sqlite"
	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/home"
)

func main() {
	apply := flag.Bool("apply", false, "actually delete; without it the sweep only reports")
	force := flag.Bool("force", false, "sweep even when the database references no assets at all (usually the wrong YTS_HOME)")
	flag.Parse()
	if err := run(*apply, *force); err != nil {
		fmt.Fprintln(os.Stderr, "sweep:", err)
		os.Exit(1)
	}
}

func run(apply, force bool) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	dir, err := home.Default()
	if err != nil {
		return err
	}
	fmt.Println("home", dir)
	store, err := sqlite.Open(ctx, sqlite.Options{Path: dir.DB()}, log)
	if err != nil {
		return err
	}
	g, gctx := errgroup.WithContext(ctx)
	writerCtx, stopWriter := context.WithCancel(gctx)
	g.Go(func() error { return store.Run(writerCtx) })

	err = sweep(gctx, store, dir.Assets(), apply, force, log)
	stopWriter()
	return errors.Join(err, g.Wait(), store.Close())
}

func sweep(ctx context.Context, store *sqlite.Store, root string, apply, force bool, log *slog.Logger) error {
	assets, err := assetstore.New(root)
	if err != nil {
		return err
	}
	// A file reachable only through a chapter's id list has no owning row
	// until this runs, and would read as garbage.
	if _, err := app.RepairAssetOwnership(ctx, store, store, store, assets, time.Now().UTC(), log); err != nil {
		return fmt.Errorf("repair asset ownership: %w", err)
	}
	report, sweepErr := app.SweepAssets(ctx, store, assets, app.SweepOptions{Apply: apply, Force: force, Now: time.Now()}, log)
	// Printed even when the sweep refused: the numbers explain the refusal.
	printReport(report, apply && sweepErr == nil)
	return sweepErr
}

func printReport(report app.SweepReport, applied bool) {
	fmt.Printf("%-14s %d\n", "files", report.Files)
	fmt.Printf("%-14s %d\n", "referenced", report.Referenced)
	fmt.Printf("%-14s %d\n", "unreferenced", report.Unreferenced)
	fmt.Printf("%-14s %d\n", "debris", report.Debris)
	if report.Unrecognised > 0 {
		fmt.Printf("%-14s %d (kept; nothing in the database describes them)\n", "unrecognised", report.Unrecognised)
		for _, rel := range report.UnrecognisedSample {
			fmt.Printf("               %s\n", rel)
		}
	}
	if applied {
		fmt.Printf("%-14s %d files, %s\n", "removed", report.Removed, humanBytes(report.Bytes))
		if report.DirsRemoved > 0 {
			fmt.Printf("%-14s %d empty directories\n", "pruned", report.DirsRemoved)
		}
		return
	}
	if report.Reclaimable() > 0 {
		fmt.Printf("\n%d files can be reclaimed. Run make sweep APPLY=1 to delete them.\n", report.Reclaimable())
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value)
}

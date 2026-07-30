package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// The 00003 migration rebuilds the assets table around a new primary key, which
// is the one operation in this schema that copies rows rather than adding to
// them. Every other test in this package opens a fresh database, where the table
// it rebuilds is empty -- so the copy is only ever exercised here.
//
// Migrations are applied by hand rather than through sqlite.Open, because the
// point is to observe the state *between* two of them.

// applyMigration runs the Up half of one migration file.
func applyMigration(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("migrations", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	up := string(raw)
	if idx := strings.Index(up, "-- +goose Down"); idx >= 0 {
		up = up[:idx]
	}
	// Comments come out before the split, because one of them contains a
	// semicolon and would otherwise cut a statement in half.
	up = stripComments(strings.ReplaceAll(up, "-- +goose Up", ""))

	for _, statement := range strings.Split(up, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, statement)
		}
	}
}

func stripComments(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func openRaw(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migrate.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAssetOwnershipMigrationPreservesOwnersAndDropsOrphans(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openRaw(t)

	applyMigration(t, db, "00001_init.sql")
	applyMigration(t, db, "00002_task_stale.sql")

	// A channel, a surviving video, and an asset row belonging to each of two
	// videos -- one of which was deleted long ago, back when a delete left its
	// rows behind. Written against the old schema, where the address is the key.
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO channels (id, slug, name, created_at, updated_at) VALUES ('ch', 'ch', 'Ch', 0, 0)`)
	exec(`INSERT INTO videos (id, channel_id, ref, title, state, chapter_count, images_per_chapter, created_at, updated_at)
	      VALUES ('v-live', 'ch', 'CH-1', 'Alive', 'draft', 1, 1, 0, 0)`)
	exec(`INSERT INTO assets (id, video_id, kind, path, size, mime, created_at)
	      VALUES ('aaa', 'v-live', 'image', 'image/aa/aaa.png', 10, 'image/png', 0)`)
	exec(`INSERT INTO assets (id, video_id, kind, path, size, mime, created_at)
	      VALUES ('bbb', 'v-gone', 'image', 'image/bb/bbb.png', 20, 'image/png', 0)`)

	applyMigration(t, db, "00003_asset_ownership.sql")

	var owner string
	if err := db.QueryRowContext(ctx, `SELECT video_id FROM assets WHERE id = 'aaa'`).Scan(&owner); err != nil {
		t.Fatalf("the surviving video's asset row did not come across: %v", err)
	}
	if owner != "v-live" {
		t.Errorf("owner = %q, want v-live", owner)
	}

	// The orphan is dropped: the new foreign key would reject it, and it is the
	// leak this migration exists to make collectable.
	var orphans int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets WHERE id = 'bbb'`).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Errorf("%d rows survived for a video that no longer exists", orphans)
	}

	// The new key admits a second owner of the same address, which the old one
	// silently discarded.
	exec(`INSERT INTO videos (id, channel_id, ref, title, state, chapter_count, images_per_chapter, created_at, updated_at)
	      VALUES ('v-two', 'ch', 'CH-2', 'Second', 'draft', 1, 1, 0, 0)`)
	exec(`INSERT INTO assets (id, video_id, kind, path, size, mime, created_at)
	      VALUES ('aaa', 'v-two', 'image', 'image/aa/aaa.png', 10, 'image/png', 0)
	      ON CONFLICT (id, video_id) DO NOTHING`)

	var owners int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets WHERE id = 'aaa'`).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 2 {
		t.Fatalf("owners of one address = %d, want 2", owners)
	}

	// And deleting a video takes only its own row with it.
	exec(`DELETE FROM videos WHERE id = 'v-live'`)
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets WHERE id = 'aaa'`).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 1 {
		t.Errorf("owners after deleting one video = %d, want 1", owners)
	}
}

package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// An asset row is one video's ownership of a content address, not the address
// itself: the primary key is (id, video_id).
//
// Both halves of that are load-bearing and neither is visible from a single
// video's point of view, which is why they are asserted here rather than left
// to the round-trip tests. Keying on the sha256 alone would give the only row
// to whichever video stored those bytes first, so a later video's asset list
// would silently omit them and a delete could not tell whose file it was
// reclaiming.
func TestOneAddressHasOneRowPerOwningVideo(t *testing.T) {
	t.Parallel()
	store, ch := seeded(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, v := range []struct{ id, ref string }{{"v-one", "DSS-1"}, {"v-two", "DSS-2"}} {
		video, err := entity.NewVideo(entity.VideoID(v.id), ch.ID, entity.Ref(v.ref), "Title", "topic", 1, 1, 4, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.CreateVideo(ctx, video); err != nil {
			t.Fatalf("CreateVideo %s: %v", v.id, err)
		}
	}

	// The same bytes, produced independently by two videos. The store holds one
	// file and the table holds two rows.
	const address = entity.AssetID("aaaa")
	for _, videoID := range []entity.VideoID{"v-one", "v-two"} {
		if err := store.PutAsset(ctx, entity.Asset{
			ID: address, VideoID: videoID, Kind: entity.AssetKindImage,
			Path: "image/aa/aaaa.png", Size: 10, MIME: "image/png", CreatedAt: now,
		}); err != nil {
			t.Fatalf("PutAsset for %s: %v", videoID, err)
		}
	}

	for _, videoID := range []entity.VideoID{"v-one", "v-two"} {
		assets, err := store.ListAssetsByVideo(ctx, videoID)
		if err != nil {
			t.Fatal(err)
		}
		if len(assets) != 1 || assets[0].ID != address {
			t.Fatalf("%s owns %d rows, want exactly the shared address", videoID, len(assets))
		}
	}

	// Deleting one owner takes only its own row. The other video still holds the
	// address, so the file is not collectable yet -- which is the question the
	// sweep asks of this table.
	reclaimable, err := store.DeleteVideo(ctx, "v-one")
	if err != nil {
		t.Fatalf("DeleteVideo: %v", err)
	}
	for _, a := range reclaimable {
		if a.ID == address {
			t.Error("an address another video still owns was reported as reclaimable")
		}
	}
	survivors, err := store.ListAssetsByVideo(ctx, "v-two")
	if err != nil {
		t.Fatal(err)
	}
	if len(survivors) != 1 {
		t.Fatalf("the surviving video owns %d rows, want 1", len(survivors))
	}
}

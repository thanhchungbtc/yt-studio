package entity_test

import (
	"testing"

	"github.com/tbui/yt-studio/domain/entity"
)

func TestChapterCountBand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name             string
		target           int
		tolerance        int
		wantMin, wantMax int
	}{
		{"the default brief", 50, 20, 40, 60},
		{"zero tolerance demands the exact count", 50, 0, 50, 50},
		{"slack rounds up so a small target can still move", 3, 20, 2, 4},
		{"a single chapter cannot go below one", 1, 50, 1, 2},
		{"the upper bound is clamped to the maximum", entity.MaxChapterCount, 20, 400, entity.MaxChapterCount},
		{"a negative tolerance is treated as none", 10, -5, 10, 10},
		{"full tolerance still cannot go below one", 4, 100, 1, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotMin, gotMax := entity.ChapterCountBand(tc.target, tc.tolerance)
			if gotMin != tc.wantMin || gotMax != tc.wantMax {
				t.Fatalf("ChapterCountBand(%d, %d) = %d..%d, want %d..%d",
					tc.target, tc.tolerance, gotMin, gotMax, tc.wantMin, tc.wantMax)
			}
		})
	}
}

// The band must always contain the target itself, or a model that hit the brief
// exactly would be rejected.
func TestChapterCountBandAlwaysContainsTheTarget(t *testing.T) {
	t.Parallel()
	for target := entity.MinChapterCount; target <= entity.MaxChapterCount; target++ {
		for _, tolerance := range []int{0, 1, 20, 100} {
			minCount, maxCount := entity.ChapterCountBand(target, tolerance)
			if target < minCount || target > maxCount {
				t.Fatalf("target %d (tolerance %d) falls outside its own band %d..%d",
					target, tolerance, minCount, maxCount)
			}
			if minCount < entity.MinChapterCount || maxCount > entity.MaxChapterCount {
				t.Fatalf("band %d..%d for target %d escapes the video bounds",
					minCount, maxCount, target)
			}
		}
	}
}

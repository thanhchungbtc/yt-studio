package entity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

func TestNewSlug(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in    string
		valid bool
	}{
		{"deep-sleep-stories", true},
		{"history", true},
		{"a1", true},
		{"channel-2024", true},
		{"a", false},
		{"", false},
		{"Deep-Sleep", false},
		{"deep_sleep", false},
		{"-leading", false},
		{"trailing-", false},
		{"double--hyphen", false},
		{"spaced out", false},
		{strings.Repeat("a", 65), false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := entity.NewSlug(tc.in)
			if tc.valid {
				if err != nil {
					t.Fatalf("NewSlug(%q) = %v, want ok", tc.in, err)
				}
				if string(got) != tc.in {
					t.Fatalf("NewSlug(%q) = %q", tc.in, got)
				}
				return
			}
			if !errors.Is(err, entity.ErrInvalidSlug) {
				t.Fatalf("NewSlug(%q) = %v, want ErrInvalidSlug", tc.in, err)
			}
		})
	}
}

func TestSlugifyName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Deep Sleep Stories":  "deep-sleep-stories",
		"History  Explained!": "history-explained",
		"  Trim Me  ":         "trim-me",
		"Channel 2024":        "channel-2024",
	}
	for in, want := range cases {
		got, err := entity.SlugifyName(in)
		if err != nil {
			t.Fatalf("SlugifyName(%q): %v", in, err)
		}
		if string(got) != want {
			t.Errorf("SlugifyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugPrefix(t *testing.T) {
	t.Parallel()
	cases := map[entity.Slug]string{
		"deep-sleep-stories": "DSS",
		"history-explained":  "HEI",
		"history":            "HIS",
		"a1":                 "A1X",
	}
	for slug, want := range cases {
		if got := slug.Prefix(); got != want {
			t.Errorf("%q.Prefix() = %q, want %q", slug, got, want)
		}
	}
}

func TestRefRoundTrip(t *testing.T) {
	t.Parallel()
	ref, err := entity.NewRef("deep-sleep-stories", 14)
	if err != nil {
		t.Fatal(err)
	}
	if string(ref) != "DSS-14" {
		t.Fatalf("ref = %q, want DSS-14", ref)
	}
	prefix, seq, err := entity.ParseRef(string(ref))
	if err != nil {
		t.Fatal(err)
	}
	if prefix != "DSS" || seq != 14 {
		t.Fatalf("ParseRef = %q,%d", prefix, seq)
	}
	if !entity.LooksLikeRef("DSS-14") {
		t.Error("DSS-14 should look like a ref")
	}
	for _, bad := range []string{"dss-14", "DSS", "DSS-", "DSS-0", "-14", "DSS-x"} {
		if entity.LooksLikeRef(bad) {
			t.Errorf("%q should not look like a ref", bad)
		}
	}
}

func TestDeterministicIDs(t *testing.T) {
	t.Parallel()
	if got := entity.NewChapterID("v1", 7); got != "v1:ch:7" {
		t.Errorf("chapter id = %q", got)
	}
	if got := entity.NewTaskID("v1", entity.TaskKindImage, 7, 1); got != "v1:image:7:1" {
		t.Errorf("task id = %q", got)
	}
	if got := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1); got != "v1:blueprint" {
		t.Errorf("video-level task id = %q", got)
	}
	if got := entity.NewTaskID("v1", entity.TaskKindScript, 3, -1); got != "v1:script:3" {
		t.Errorf("per-chapter task id = %q", got)
	}
}

// Every task kind must map to exactly one valid pool, or admission control has
// a hole.
func TestEveryTaskKindHasAPool(t *testing.T) {
	t.Parallel()
	for _, kind := range entity.AllTaskKinds {
		if !kind.Valid() {
			t.Errorf("%q is in AllTaskKinds but not Valid", kind)
		}
		if pool := kind.Pool(); !pool.Valid() {
			t.Errorf("%q maps to invalid pool %q", kind, pool)
		}
	}
	if entity.TaskKind("nonsense").Valid() {
		t.Error("an unknown kind reported itself valid")
	}
}

func TestEveryPoolHasALimitKeyAndIndex(t *testing.T) {
	t.Parallel()
	seen := map[int]entity.Pool{}
	for _, pool := range entity.AllPools {
		idx := pool.Index()
		if idx < 0 || idx >= entity.NumPools {
			t.Fatalf("pool %q has index %d", pool, idx)
		}
		if other, dup := seen[idx]; dup {
			t.Fatalf("pools %q and %q share index %d", pool, other, idx)
		}
		seen[idx] = pool
		if key := entity.PoolLimitKey(pool); key == "" {
			t.Errorf("pool %q has no settings key", pool)
		}
		if entity.DefaultPoolLimit(pool) < 1 {
			t.Errorf("pool %q has a default limit below 1", pool)
		}
	}
	if entity.Pool("nope").Index() != -1 {
		t.Error("unknown pool did not report index -1")
	}
}

func TestTaskStateClassification(t *testing.T) {
	t.Parallel()
	for _, state := range entity.AllTaskStates {
		if !state.Valid() {
			t.Errorf("%q is in AllTaskStates but not Valid", state)
		}
		// Terminal and Open are complements by construction: a task the scheduler
		// must still carry is exactly one it has not finished with.
		if state.Terminal() == state.Open() {
			t.Errorf("%q: Terminal()=%v Open()=%v, expected complements", state, state.Terminal(), state.Open())
		}
	}
}

func TestVideoStateMachine(t *testing.T) {
	t.Parallel()
	if !entity.VideoStateDraft.CanTransitionTo(entity.VideoStateRunning) {
		t.Error("draft -> running should be allowed")
	}
	if entity.VideoStateCompleted.CanTransitionTo(entity.VideoStateRunning) {
		t.Error("completed -> running should not be allowed")
	}
	if !entity.VideoStateFailed.CanTransitionTo(entity.VideoStateRunning) {
		t.Error("failed -> running (a retry) should be allowed")
	}
	if !entity.VideoStateRunning.CanTransitionTo(entity.VideoStateRunning) {
		t.Error("a state should always allow itself")
	}
	for _, s := range entity.AllVideoStates {
		if !s.Valid() {
			t.Errorf("%q is in AllVideoStates but not Valid", s)
		}
	}
}

// Every outcome type must be handled at every type-switch site. `exhaustive`
// cannot check a type switch, so the sealed set is asserted here and each site
// has its own table-driven test.
func TestOutcomeSetIsSealedAndComplete(t *testing.T) {
	t.Parallel()
	outcomes := entity.AllTaskOutcomes()
	if len(outcomes) != 3 {
		t.Fatalf("AllTaskOutcomes returned %d cases; update every type switch", len(outcomes))
	}
	seen := map[string]bool{}
	for _, o := range outcomes {
		switch typed := o.(type) {
		case entity.Success:
			seen["success"] = true
		case entity.Failed:
			seen["failed"] = true
		case entity.AwaitingApproval:
			seen["awaiting"] = true
		default:
			t.Fatalf("unhandled outcome %T", typed)
		}
	}
	for _, want := range []string{"success", "failed", "awaiting"} {
		if !seen[want] {
			t.Errorf("outcome %q missing from AllTaskOutcomes", want)
		}
	}
}

func TestAssetKindsAreComplete(t *testing.T) {
	t.Parallel()
	for _, kind := range entity.AllAssetKinds {
		if !kind.Valid() {
			t.Errorf("%q is in AllAssetKinds but not Valid", kind)
		}
		if kind.Ext() == ".bin" {
			t.Errorf("%q has no declared extension", kind)
		}
		if kind.MIME() == "application/octet-stream" {
			t.Errorf("%q has no declared MIME type", kind)
		}
	}
}

func TestNewAssetValidation(t *testing.T) {
	t.Parallel()
	hash := strings.Repeat("a", 64)
	now := time.Unix(0, 0)
	if _, err := entity.NewAsset(entity.AssetID(hash), "v1", nil, entity.AssetKindImage, "image/aa/x.png", 10, "test", now); err != nil {
		t.Fatalf("valid asset rejected: %v", err)
	}
	for _, tc := range []struct {
		name string
		id   string
		kind entity.AssetKind
		path string
		size int64
	}{
		{"short id", "abc", entity.AssetKindImage, "p", 1},
		{"bad kind", hash, "nope", "p", 1},
		{"no path", hash, entity.AssetKindImage, "", 1},
		{"negative size", hash, entity.AssetKindImage, "p", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := entity.NewAsset(entity.AssetID(tc.id), "v1", nil, tc.kind, tc.path, tc.size, "test", now); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestNewVideoValidation(t *testing.T) {
	t.Parallel()
	now := time.Unix(0, 0)
	if _, err := entity.NewVideo("v1", "c1", "DSS-1", "Title", "topic", 50, 2, 0, now); err != nil {
		t.Fatalf("valid video rejected: %v", err)
	}
	bad := []struct {
		name     string
		ref      entity.Ref
		title    string
		chapters int
		images   int
	}{
		{"bad ref", "nope", "Title", 1, 1},
		{"no title", "DSS-1", "  ", 1, 1},
		{"zero chapters", "DSS-1", "Title", 0, 1},
		{"too many chapters", "DSS-1", "Title", entity.MaxChapterCount + 1, 1},
		{"zero images", "DSS-1", "Title", 1, 0},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := entity.NewVideo("v1", "c1", tc.ref, tc.title, "", tc.chapters, tc.images, 0, now); !errors.Is(err, entity.ErrInvalidVideo) {
				t.Fatalf("err = %v, want ErrInvalidVideo", err)
			}
		})
	}
}

// A fresh database must boot with a complete, valid settings set and no
// null-handling anywhere.
func TestDefaultSettingsAreValidAndComplete(t *testing.T) {
	t.Parallel()
	defaults := entity.DefaultSettings()
	seen := make(map[entity.SettingKey]bool, len(defaults))
	for _, s := range defaults {
		if err := s.Validate(); err != nil {
			t.Errorf("default setting %q is invalid: %v", s.Key, err)
		}
		if seen[s.Key] {
			t.Errorf("duplicate default setting %q", s.Key)
		}
		seen[s.Key] = true
		if s.Group == "" {
			t.Errorf("setting %q has no group; the settings screen needs one", s.Key)
		}
		if s.Description == "" {
			t.Errorf("setting %q has no description", s.Key)
		}
	}
	for _, pool := range entity.AllPools {
		if !seen[entity.PoolLimitKey(pool)] {
			t.Errorf("pool %q has no seeded limit row", pool)
		}
	}
	for _, gate := range entity.AllGateKinds {
		if !seen[entity.GateEnabledKey(gate)] {
			t.Errorf("gate %q has no seeded toggle row", gate)
		}
	}
}

func TestSettingTypedAccessors(t *testing.T) {
	t.Parallel()
	s := entity.Setting{Key: "k", Value: "42", Type: entity.SettingTypeInt, Min: 1, Max: 100}
	if n, err := s.Int(); err != nil || n != 42 {
		t.Fatalf("Int() = %d, %v", n, err)
	}
	if d, err := s.Duration(); err != nil || d != 42*time.Millisecond {
		t.Fatalf("Duration() = %v, %v", d, err)
	}
	s.Value = "1000"
	if err := s.Validate(); err == nil {
		t.Fatal("out-of-range value passed validation")
	}
	b := entity.Setting{Key: "k", Value: "true", Type: entity.SettingTypeBool}
	if got, err := b.Bool(); err != nil || !got {
		t.Fatalf("Bool() = %v, %v", got, err)
	}
	b.Value = "maybe"
	if err := b.Validate(); err == nil {
		t.Fatal("unparsable boolean passed validation")
	}
}

func TestSettingOptionsConstrainTheValue(t *testing.T) {
	t.Parallel()
	s := entity.Setting{
		Key:     entity.SettingProviderComposer,
		Value:   "ffmpeg",
		Type:    entity.SettingTypeString,
		Options: []string{"mock", "ffmpeg"},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("a registered backend was rejected: %v", err)
	}

	// A near miss is the case that matters: without the constraint it would
	// validate, persist, and silently run a different backend.
	s.Value = "ffmpg"
	err := s.Validate()
	if !errors.Is(err, entity.ErrInvalidSetting) {
		t.Fatalf("misspelled backend passed validation: %v", err)
	}
	if !strings.Contains(err.Error(), "mock, ffmpeg") {
		t.Fatalf("error does not name the legal values: %v", err)
	}

	free := entity.Setting{Key: "k", Value: "anything", Type: entity.SettingTypeString}
	if err := free.Validate(); err != nil {
		t.Fatalf("an unconstrained setting was rejected: %v", err)
	}
}

func TestChapterNaturalKey(t *testing.T) {
	t.Parallel()
	c, err := entity.NewChapter("v1", 7, "Title", "summary", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.NaturalKey("DSS-14"); got != "DSS-14#7" {
		t.Fatalf("natural key = %q, want DSS-14#7", got)
	}
	if _, err := entity.NewChapter("v1", 0, "Title", "", time.Unix(0, 0)); !errors.Is(err, entity.ErrInvalidChapter) {
		t.Fatalf("ordinal 0 accepted: %v", err)
	}
}

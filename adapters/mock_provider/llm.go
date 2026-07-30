package mockprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// LLM is the mock text backend. It writes real JSON and text assets to the
// store and returns their content addresses.
type LLM struct {
	store  provider.AssetStore
	lookup ContextLookup
	tuning Tuning

	// singleflight collapses concurrent callers onto one production; the cache
	// serves every later caller. Both halves are needed: singleflight alone
	// deduplicates only calls that overlap in time.
	prompts singleflight.Group
	cacheMu sync.RWMutex
	cache   map[entity.VideoID][]provider.ImagePrompt
}

var _ provider.LLMProvider = (*LLM)(nil)

// NewLLM constructs the mock. Every dependency is an explicit parameter.
func NewLLM(store provider.AssetStore, lookup ContextLookup, tuning Tuning) *LLM {
	return &LLM{
		store:  store,
		lookup: lookup,
		tuning: tuning,
		cache:  make(map[entity.VideoID][]provider.ImagePrompt, 8),
	}
}

// Blueprint outlines a whole video: one call, one unit of work.
func (l *LLM) Blueprint(ctx context.Context, req provider.BlueprintRequest) (provider.Blueprint, error) {
	if err := simulate(ctx, l.tuning, 4); err != nil {
		return provider.Blueprint{}, err
	}
	seed := seedOf(string(req.VideoID), req.Title, req.Topic, strconv.Itoa(req.ChapterCount))

	bp := provider.Blueprint{
		Title: req.Title,
		Summary: fmt.Sprintf("%s. A %d-chapter narration in a %s register, produced for %s.",
			firstNonEmpty(req.Topic, req.Title), req.ChapterCount,
			firstNonEmpty(req.Style.Tone, "neutral"), req.ChannelSlug),
		Chapters: make([]provider.BlueprintChapter, 0, req.ChapterCount),
	}
	for i := 1; i <= req.ChapterCount; i++ {
		cr := deterministic(seed ^ uint64(i)*0x100000001B3) //nolint:gosec // deterministic mixing
		bp.Chapters = append(bp.Chapters, provider.BlueprintChapter{
			Ordinal:        i,
			Title:          chapterTitle(cr, req.Topic, i),
			Summary:        chapterSummary(cr, req.Topic, i),
			EstimatedWords: mockChapterWords(req),
		})
	}

	doc := struct {
		Video    string                      `json:"video"`
		Ref      entity.Ref                  `json:"ref"`
		Title    string                      `json:"title"`
		Summary  string                      `json:"summary"`
		Chapters []provider.BlueprintChapter `json:"chapters"`
	}{string(req.VideoID), req.VideoRef, bp.Title, bp.Summary, bp.Chapters}

	id, err := l.putJSON(ctx, entity.AssetKindBlueprint, doc)
	if err != nil {
		return provider.Blueprint{}, err
	}
	bp.AssetID = id
	return bp, nil
}

// mockChapterWords is the flat per-chapter budget the mock plans with. It does
// not vary by pacing the way a real outline does; it exists so the field is
// populated and the pipeline is exercised end to end.
func mockChapterWords(req provider.BlueprintRequest) int {
	if req.Style.WordsPerChapter > 0 {
		return req.Style.WordsPerChapter
	}
	return entity.DefaultWordsPerChapter
}

// Script writes exactly one chapter's narration.
func (l *LLM) Script(ctx context.Context, req provider.ScriptRequest) (provider.Script, error) {
	if err := simulate(ctx, l.tuning, 1); err != nil {
		return provider.Script{}, err
	}
	words := req.TargetWords
	if words <= 0 {
		words = req.Style.WordsPerChapter
	}
	if words <= 0 {
		words = entity.DefaultWordsPerChapter
	}
	seed := seedOf(string(req.VideoID), strconv.Itoa(req.Ordinal), req.ChapterTitle)
	text := narration(seed, req.ChapterTitle, req.ChapterSummary, req.Style.Tone, words)

	stored, err := l.store.Put(ctx, entity.AssetKindScript, strings.NewReader(text))
	if err != nil {
		return provider.Script{}, fmt.Errorf("store script: %w", err)
	}
	return provider.Script{
		Text:      text,
		WordCount: len(strings.Fields(text)),
		AssetID:   stored.ID,
	}, nil
}

// ImagePrompts returns every chapter's prompts for one video.
//
// All prompts come from one generation — better cross-chapter visual
// coherence, and it avoids re-sending blueprint context N times. The DAG still
// holds N individually retryable per-chapter tasks; singleflight is what
// collapses them onto one production and serves the rest from cache.
func (l *LLM) ImagePrompts(ctx context.Context, videoID entity.VideoID) ([]provider.ImagePrompt, error) {
	if cached, ok := l.cached(videoID); ok {
		return cached, nil
	}
	key := string(videoID) + "|image_prompts"
	v, err, _ := l.prompts.Do(key, func() (any, error) {
		// A caller that queued behind the production must not trigger another.
		if cached, ok := l.cached(videoID); ok {
			return cached, nil
		}
		vc, err := l.lookup(ctx, videoID)
		if err != nil {
			return nil, fmt.Errorf("resolve video context: %w", err)
		}
		if err := simulate(ctx, l.tuning, 3); err != nil {
			return nil, err
		}
		perChapter := vc.ImagesPerChapter
		if perChapter <= 0 {
			perChapter = 1
		}
		out := make([]provider.ImagePrompt, 0, len(vc.Chapters)*perChapter)
		for _, ch := range vc.Chapters {
			for j := range perChapter {
				seed := seedOf(string(videoID), strconv.Itoa(ch.Ordinal), strconv.Itoa(j))
				out = append(out, provider.ImagePrompt{
					Ordinal: ch.Ordinal,
					Index:   j,
					Prompt:  imagePrompt(seed, ch.Title, ch.Summary, vc.Style.ImageStyle),
				})
			}
		}
		l.cacheMu.Lock()
		l.cache[videoID] = out
		l.cacheMu.Unlock()
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	prompts, ok := v.([]provider.ImagePrompt)
	if !ok {
		return nil, fmt.Errorf("mock provider: unexpected prompt cache type %T", v)
	}
	return cloneprompts(prompts), nil
}

// cached returns a caller-owned copy of the batch if it has been produced.
func (l *LLM) cached(videoID entity.VideoID) ([]provider.ImagePrompt, bool) {
	l.cacheMu.RLock()
	prompts, ok := l.cache[videoID]
	l.cacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	return cloneprompts(prompts), true
}

// cloneprompts hands every caller its own slice; the cached one never escapes.
func cloneprompts(in []provider.ImagePrompt) []provider.ImagePrompt {
	out := make([]provider.ImagePrompt, len(in))
	copy(out, in)
	return out
}

// Forget drops a video's coalesced prompt batch, so a retry regenerates it
// rather than replaying the output the operator rejected.
func (l *LLM) Forget(videoID entity.VideoID) {
	l.cacheMu.Lock()
	delete(l.cache, videoID)
	l.cacheMu.Unlock()
	l.prompts.Forget(string(videoID) + "|image_prompts")
}

// Metadata writes the YouTube-facing listing.
func (l *LLM) Metadata(ctx context.Context, req provider.MetadataRequest) (provider.Metadata, error) {
	if err := simulate(ctx, l.tuning, 2); err != nil {
		return provider.Metadata{}, err
	}
	seed := seedOf(string(req.VideoID), req.Title, req.Topic)
	r := deterministic(seed)

	var desc strings.Builder
	desc.WriteString(req.Title)
	desc.WriteString("\n\n")
	desc.WriteString(firstNonEmpty(req.Topic, req.Title))
	desc.WriteString(".\n\nChapters:\n")
	clock := 0
	for _, ch := range req.Chapters {
		desc.WriteString(formatTimecode(clock))
		desc.WriteByte(' ')
		desc.WriteString(ch.Title)
		desc.WriteByte('\n')
		clock += 180 + r.IntN(120)
	}

	md := entity.Metadata{
		Title:       truncate(req.Title, 100),
		Description: desc.String(),
		Tags:        tagsFor(r, req.Topic, req.Style),
		CategoryID:  "22",
		Privacy:     "private",
	}
	id, err := l.putJSON(ctx, entity.AssetKindMetadata, md)
	if err != nil {
		return provider.Metadata{}, err
	}
	return provider.Metadata{Metadata: md, AssetID: id}, nil
}

func (l *LLM) putJSON(ctx context.Context, kind entity.AssetKind, v any) (entity.AssetID, error) {
	// Indented output keeps blueprints readable in the operator UI, and the
	// encoding is stable so the content address is stable.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("encode %s: %w", kind, err)
	}
	stored, err := l.store.Put(ctx, kind, &buf)
	if err != nil {
		return "", fmt.Errorf("store %s: %w", kind, err)
	}
	return stored.ID, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func formatTimecode(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	var b strings.Builder
	b.Grow(8)
	b.WriteString(strconv.Itoa(h))
	b.WriteByte(':')
	if m < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.Itoa(m))
	b.WriteByte(':')
	if s < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.Itoa(s))
	return b.String()
}

package registry_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/tbui/yt-studio/adapters/registry"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// stubTTS reports which backend was reached.
type stubTTS struct {
	name   string
	called int
}

func (s *stubTTS) Speak(context.Context, provider.SpeakRequest) (entity.AssetID, error) {
	s.called++
	return entity.AssetID(s.name), nil
}

// stubLLM is the minimum that satisfies the LLM port, plus the prompt cache.
type stubLLM struct{ forgotten []entity.VideoID }

func (s *stubLLM) Blueprint(context.Context, provider.BlueprintRequest) (provider.Blueprint, error) {
	return provider.Blueprint{}, nil
}
func (s *stubLLM) Script(context.Context, provider.ScriptRequest) (provider.Script, error) {
	return provider.Script{}, nil
}
func (s *stubLLM) ImagePrompts(context.Context, entity.VideoID) ([]provider.ImagePrompt, error) {
	return nil, nil
}
func (s *stubLLM) Metadata(context.Context, provider.MetadataRequest) (provider.Metadata, error) {
	return provider.Metadata{}, nil
}
func (s *stubLLM) Forget(videoID entity.VideoID) { s.forgotten = append(s.forgotten, videoID) }

// uncachedLLM keeps no prompt cache, so it has no Forget at all.
type uncachedLLM struct{}

func (uncachedLLM) Blueprint(context.Context, provider.BlueprintRequest) (provider.Blueprint, error) {
	return provider.Blueprint{}, nil
}
func (uncachedLLM) Script(context.Context, provider.ScriptRequest) (provider.Script, error) {
	return provider.Script{}, nil
}
func (uncachedLLM) ImagePrompts(context.Context, entity.VideoID) ([]provider.ImagePrompt, error) {
	return nil, nil
}
func (uncachedLLM) Metadata(context.Context, provider.MetadataRequest) (provider.Metadata, error) {
	return provider.Metadata{}, nil
}

func TestSelectionFollowsTheSettingsRow(t *testing.T) {
	t.Parallel()
	selected := "mock"
	reg := registry.New(func(entity.SettingKey) string { return selected })

	mock := &stubTTS{name: "mock"}
	other := &stubTTS{name: "other"}
	reg.RegisterTTS("mock", mock)
	reg.RegisterTTS("other", other)

	tts := reg.TTS()
	if id, err := tts.Speak(context.Background(), provider.SpeakRequest{}); err != nil || id != "mock" {
		t.Fatalf("Speak() = %q, %v; want mock", id, err)
	}

	// The row is re-read per call, which is what makes a settings edit apply to
	// the next task rather than the next restart.
	selected = "other"
	if id, err := tts.Speak(context.Background(), provider.SpeakRequest{}); err != nil || id != "other" {
		t.Fatalf("after the edit, Speak() = %q, %v; want other", id, err)
	}
	if mock.called != 1 || other.called != 1 {
		t.Fatalf("calls landed wrong: mock=%d other=%d", mock.called, other.called)
	}
}

func TestUnknownBackendIsAnErrorNotAFallback(t *testing.T) {
	t.Parallel()
	reg := registry.New(func(entity.SettingKey) string { return "elevenlabs" })
	reg.RegisterTTS("mock", &stubTTS{name: "mock"})

	_, err := reg.TTS().Speak(context.Background(), provider.SpeakRequest{})
	if !errors.Is(err, registry.ErrUnknownBackend) {
		t.Fatalf("unregistered backend silently fell back: %v", err)
	}
}

func TestOptionsReportRegisteredBackends(t *testing.T) {
	t.Parallel()
	reg := registry.New(func(entity.SettingKey) string { return "mock" })
	reg.RegisterComposer("mock", nil)
	reg.RegisterComposer("ffmpeg", nil)
	reg.RegisterTTS("mock", &stubTTS{})

	options := reg.Options()
	if got := options[entity.SettingProviderComposer]; !slices.Equal(got, []string{"ffmpeg", "mock"}) {
		t.Fatalf("composer options = %v; want sorted [ffmpeg mock]", got)
	}
	if got := options[entity.SettingProviderTTS]; !slices.Equal(got, []string{"mock"}) {
		t.Fatalf("tts options = %v", got)
	}
	// A port with nothing registered still needs an entry, or the settings row
	// would be silently unconstrained.
	if got, ok := options[entity.SettingProviderUploader]; !ok || len(got) != 0 {
		t.Fatalf("uploader options = %v, present=%v; want an empty entry", got, ok)
	}
}

func TestPromptCacheReachesTheSelectedBackend(t *testing.T) {
	t.Parallel()
	selected := "cached"
	reg := registry.New(func(entity.SettingKey) string { return selected })
	cached := &stubLLM{}
	reg.RegisterLLM("cached", cached)
	reg.RegisterLLM("plain", uncachedLLM{})

	reg.PromptCache().Forget("v1")
	if !slices.Equal(cached.forgotten, []entity.VideoID{"v1"}) {
		t.Fatalf("forgotten = %v; want [v1]", cached.forgotten)
	}

	// A backend that keeps no cache has nothing to drop, and an unknown one
	// cannot be reached at all. Neither may panic, and neither may reach the
	// backend that is no longer selected.
	selected = "plain"
	reg.PromptCache().Forget("v2")
	selected = "missing"
	reg.PromptCache().Forget("v3")
	if !slices.Equal(cached.forgotten, []entity.VideoID{"v1"}) {
		t.Fatalf("forgotten = %v; want the deselected backend to be untouched", cached.forgotten)
	}
}

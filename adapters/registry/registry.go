// Package registry routes each provider port to the backend named by its
// settings row.
//
// Selection is one mechanism used once per port rather than a bespoke wrapper per
// port: a backend is registered under a name, the settings row names one, and
// the router resolves it per call so an edit on the settings screen applies to
// the next task instead of the next restart.
//
// A name that is not registered is an error, never a fallback. Silently
// downgrading to a different backend because a settings value was misspelled is
// the failure mode this package exists to prevent — the operator finds out
// from a startup error, not from watching the output.
package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnknownBackend reports a settings value that names no registered backend.
// Load and Set reject it, so reaching this from a provider call means the row
// was changed underneath the daemon.
var ErrUnknownBackend = errors.New("unknown provider backend")

// Selected reads the backend name for one settings key. It is called per
// provider call, which is what makes a settings edit take effect live.
type Selected func(entity.SettingKey) string

// port holds every backend registered for one provider interface.
type port[T any] struct {
	key      entity.SettingKey
	impls    map[string]T
	selected Selected
}

func newPort[T any](key entity.SettingKey, selected Selected) *port[T] {
	return &port[T]{key: key, impls: make(map[string]T, 2), selected: selected}
}

func (p *port[T]) register(name string, impl T) { p.impls[name] = impl }

// pick resolves the configured backend.
func (p *port[T]) pick() (T, error) {
	name := p.selected(p.key)
	impl, ok := p.impls[name]
	if !ok {
		var zero T
		return zero, fmt.Errorf("%w: %s = %q, expected one of %v", ErrUnknownBackend, p.key, name, p.names())
	}
	return impl, nil
}

// names lists the registered backends, sorted so the settings screen and the
// error above are stable rather than map-ordered.
func (p *port[T]) names() []string {
	out := make([]string, 0, len(p.impls))
	for name := range p.impls {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Registry owns every provider port's backends.
type Registry struct {
	llm           *port[provider.LLMProvider]
	tts           *port[provider.TTSProvider]
	image         *port[provider.ImageProvider]
	composer      *port[provider.VideoComposer]
	thumbnail     *port[provider.ThumbnailBuilder]
	thumbnailIcon *port[provider.ThumbnailIconGenerator]
	uploader      *port[provider.Uploader]
}

// New creates an empty registry bound to a settings reader.
func New(selected Selected) *Registry {
	return &Registry{
		llm:       newPort[provider.LLMProvider](entity.SettingProviderLLM, selected),
		tts:       newPort[provider.TTSProvider](entity.SettingProviderTTS, selected),
		image:     newPort[provider.ImageProvider](entity.SettingProviderImage, selected),
		composer:  newPort[provider.VideoComposer](entity.SettingProviderComposer, selected),
		thumbnail: newPort[provider.ThumbnailBuilder](entity.SettingProviderThumbnail, selected),
		thumbnailIcon: newPort[provider.ThumbnailIconGenerator](
			entity.SettingProviderThumbnailIcon, selected),
		uploader: newPort[provider.Uploader](entity.SettingProviderUploader, selected),
	}
}

// RegisterLLM adds a named blueprint, script, prompt and metadata backend.
func (r *Registry) RegisterLLM(name string, impl provider.LLMProvider) { r.llm.register(name, impl) }

// RegisterTTS adds a named narration backend.
func (r *Registry) RegisterTTS(name string, impl provider.TTSProvider) { r.tts.register(name, impl) }

// RegisterImage adds a named still backend.
func (r *Registry) RegisterImage(name string, impl provider.ImageProvider) {
	r.image.register(name, impl)
}

// RegisterComposer adds a named clip and concat backend.
func (r *Registry) RegisterComposer(name string, impl provider.VideoComposer) {
	r.composer.register(name, impl)
}

// RegisterThumbnail adds a named thumbnail backend.
func (r *Registry) RegisterThumbnail(name string, impl provider.ThumbnailBuilder) {
	r.thumbnail.register(name, impl)
}

// RegisterThumbnailIcon adds a named backend for the thumbnail's grid icons.
func (r *Registry) RegisterThumbnailIcon(name string, impl provider.ThumbnailIconGenerator) {
	r.thumbnailIcon.register(name, impl)
}

// RegisterUploader adds a named publishing backend.
func (r *Registry) RegisterUploader(name string, impl provider.Uploader) {
	r.uploader.register(name, impl)
}

// Options reports the registered backend names per settings key, for
// service.Settings.Constrain. This is what makes the settings screen a dropdown
// of backends that genuinely exist in this binary.
//
//nolint:exhaustive // deliberately partial: only the provider.* keys name a backend
func (r *Registry) Options() map[entity.SettingKey][]string {
	return map[entity.SettingKey][]string{
		entity.SettingProviderLLM:           r.llm.names(),
		entity.SettingProviderTTS:           r.tts.names(),
		entity.SettingProviderImage:         r.image.names(),
		entity.SettingProviderComposer:      r.composer.names(),
		entity.SettingProviderThumbnail:     r.thumbnail.names(),
		entity.SettingProviderThumbnailIcon: r.thumbnailIcon.names(),
		entity.SettingProviderUploader:      r.uploader.names(),
	}
}

// PromptCache drops a video's coalesced image-prompt batch. It is declared here
// rather than imported from app so this package stays below it; the router
// satisfies app.PromptCacheInvalidator structurally.
type PromptCache interface {
	Forget(videoID entity.VideoID)
}

// LLM returns the router for the LLM port.
func (r *Registry) LLM() provider.LLMProvider { return llmRouter{r.llm} }

// PromptCache returns the invalidator for whichever LLM backend is selected.
func (r *Registry) PromptCache() PromptCache { return llmRouter{r.llm} }

// TTS returns the router for the narration port.
func (r *Registry) TTS() provider.TTSProvider { return ttsRouter{r.tts} }

// Image returns the router for the still port.
func (r *Registry) Image() provider.ImageProvider { return imageRouter{r.image} }

// Composer returns the router for the composition port.
func (r *Registry) Composer() provider.VideoComposer { return composerRouter{r.composer} }

// Thumbnail returns the router for the thumbnail port.
func (r *Registry) Thumbnail() provider.ThumbnailBuilder { return thumbnailRouter{r.thumbnail} }

// ThumbnailIcon returns the router for the thumbnail icon port.
func (r *Registry) ThumbnailIcon() provider.ThumbnailIconGenerator {
	return thumbnailIconRouter{r.thumbnailIcon}
}

// Uploader returns the router for the publishing port.
func (r *Registry) Uploader() provider.Uploader { return uploaderRouter{r.uploader} }

// The routers below are the only hand-written glue selection needs: one
// delegating method per port method, resolving the backend per call.

type llmRouter struct{ p *port[provider.LLMProvider] }

var _ provider.LLMProvider = llmRouter{}

func (r llmRouter) Blueprint(ctx context.Context, req provider.BlueprintRequest) (provider.Blueprint, error) {
	impl, err := r.p.pick()
	if err != nil {
		return provider.Blueprint{}, err
	}
	return impl.Blueprint(ctx, req)
}

func (r llmRouter) Script(ctx context.Context, req provider.ScriptRequest) (provider.Script, error) {
	impl, err := r.p.pick()
	if err != nil {
		return provider.Script{}, err
	}
	return impl.Script(ctx, req)
}

func (r llmRouter) ImagePrompts(ctx context.Context, videoID entity.VideoID) ([]provider.ImagePrompt, error) {
	impl, err := r.p.pick()
	if err != nil {
		return nil, err
	}
	return impl.ImagePrompts(ctx, videoID)
}

func (r llmRouter) Metadata(ctx context.Context, req provider.MetadataRequest) (provider.Metadata, error) {
	impl, err := r.p.pick()
	if err != nil {
		return provider.Metadata{}, err
	}
	return impl.Metadata(ctx, req)
}

func (r llmRouter) ThumbnailPlan(ctx context.Context, req provider.ThumbnailPlanRequest) (provider.ThumbnailPlan, error) {
	impl, err := r.p.pick()
	if err != nil {
		return provider.ThumbnailPlan{}, err
	}
	return impl.ThumbnailPlan(ctx, req)
}

// Forget invalidates the selected backend's coalesced prompt batch, for the
// backends that keep one. A backend without a cache has nothing to drop, and a
// misconfigured row is already failing every generate call.
func (r llmRouter) Forget(videoID entity.VideoID) {
	impl, err := r.p.pick()
	if err != nil {
		return
	}
	if cache, ok := impl.(PromptCache); ok {
		cache.Forget(videoID)
	}
}

type ttsRouter struct{ p *port[provider.TTSProvider] }

var _ provider.TTSProvider = ttsRouter{}

func (r ttsRouter) Speak(ctx context.Context, req provider.SpeakRequest) (entity.AssetID, error) {
	impl, err := r.p.pick()
	if err != nil {
		return "", err
	}
	return impl.Speak(ctx, req)
}

type imageRouter struct{ p *port[provider.ImageProvider] }

var _ provider.ImageProvider = imageRouter{}

func (r imageRouter) Generate(ctx context.Context, req provider.ImageRequest) (entity.AssetID, error) {
	impl, err := r.p.pick()
	if err != nil {
		return "", err
	}
	return impl.Generate(ctx, req)
}

type composerRouter struct{ p *port[provider.VideoComposer] }

var _ provider.VideoComposer = composerRouter{}

func (r composerRouter) Clip(ctx context.Context, req provider.ClipRequest) (entity.AssetID, error) {
	impl, err := r.p.pick()
	if err != nil {
		return "", err
	}
	return impl.Clip(ctx, req)
}

func (r composerRouter) Concat(ctx context.Context, req provider.ConcatRequest) (entity.AssetID, error) {
	impl, err := r.p.pick()
	if err != nil {
		return "", err
	}
	return impl.Concat(ctx, req)
}

type thumbnailRouter struct {
	p *port[provider.ThumbnailBuilder]
}

var _ provider.ThumbnailBuilder = thumbnailRouter{}

func (r thumbnailRouter) Build(ctx context.Context, req provider.ThumbnailRequest) (entity.AssetID, error) {
	impl, err := r.p.pick()
	if err != nil {
		return "", err
	}
	return impl.Build(ctx, req)
}

type thumbnailIconRouter struct {
	p *port[provider.ThumbnailIconGenerator]
}

var _ provider.ThumbnailIconGenerator = thumbnailIconRouter{}

func (r thumbnailIconRouter) Icon(ctx context.Context, req provider.ThumbnailIconRequest) (entity.AssetID, error) {
	impl, err := r.p.pick()
	if err != nil {
		return "", err
	}
	return impl.Icon(ctx, req)
}

type uploaderRouter struct{ p *port[provider.Uploader] }

var _ provider.Uploader = uploaderRouter{}

func (r uploaderRouter) Upload(ctx context.Context, req provider.UploadRequest) (entity.UploadRecord, error) {
	impl, err := r.p.pick()
	if err != nil {
		return entity.UploadRecord{}, err
	}
	return impl.Upload(ctx, req)
}

package http

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/service"
)

// SettingsOutput is the settings table.
type SettingsOutput struct {
	Body struct {
		Settings []SettingDTO `json:"settings"`
	}
}

// SettingOutput is one settings row.
type SettingOutput struct {
	Body SettingDTO
}

// UpdateSettingInput edits one settings row.
type UpdateSettingInput struct {
	Key  string `path:"key" doc:"Settings key, e.g. pool.image.limit"`
	Body struct {
		Value string `json:"value" required:"true"`
	}
}

func getSettings(settings *service.Settings) func(context.Context, *struct{}) (*SettingsOutput, error) {
	return func(_ context.Context, _ *struct{}) (*SettingsOutput, error) {
		rows := app.ListSettings(settings)
		out := &SettingsOutput{}
		out.Body.Settings = make([]SettingDTO, 0, len(rows))
		for _, s := range rows {
			out.Body.Settings = append(out.Body.Settings, settingFrom(s))
		}
		return out, nil
	}
}

func putSetting(
	settings *service.Settings,
	pools app.PoolLimiter,
	coalesce app.CoalesceSetter,
	level *slog.LevelVar,
) func(context.Context, *UpdateSettingInput) (*SettingOutput, error) {
	return func(ctx context.Context, in *UpdateSettingInput) (*SettingOutput, error) {
		s, err := app.UpdateSetting(ctx, settings, pools, coalesce, level,
			entity.SettingKey(in.Key), in.Body.Value)
		if err != nil {
			return nil, mapError(err)
		}
		return &SettingOutput{Body: settingFrom(s)}, nil
	}
}

// PresetsOutput is the presets this build ships with.
type PresetsOutput struct {
	Body struct {
		Presets []PresetDTO `json:"presets"`
	}
}

// ApplyPresetInput names the preset to write.
type ApplyPresetInput struct {
	Name string `path:"name" doc:"Preset name, e.g. mock"`
}

// ApplyPresetOutput is the rows the preset changed.
//
// Only the rows that moved: a preset already in force changes nothing and says
// so with an empty list, which is also what lets the client patch its cache from
// the response instead of refetching the table.
type ApplyPresetOutput struct {
	Body struct {
		Settings []SettingDTO `json:"settings"`
	}
}

func getPresets(_ context.Context, _ *struct{}) (*PresetsOutput, error) {
	rows := app.ListPresets()
	out := &PresetsOutput{}
	out.Body.Presets = make([]PresetDTO, 0, len(rows))
	for _, p := range rows {
		out.Body.Presets = append(out.Body.Presets, presetFrom(p))
	}
	return out, nil
}

func applyPreset(
	settings *service.Settings,
	pools app.PoolLimiter,
	coalesce app.CoalesceSetter,
	level *slog.LevelVar,
) func(context.Context, *ApplyPresetInput) (*ApplyPresetOutput, error) {
	return func(ctx context.Context, in *ApplyPresetInput) (*ApplyPresetOutput, error) {
		changed, err := app.ApplyPreset(ctx, settings, pools, coalesce, level, in.Name)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ApplyPresetOutput{}
		out.Body.Settings = make([]SettingDTO, 0, len(changed))
		for _, s := range changed {
			out.Body.Settings = append(out.Body.Settings, settingFrom(s))
		}
		return out, nil
	}
}

func registerSettingRoutes(
	api huma.API,
	settings *service.Settings,
	pools app.PoolLimiter,
	coalesce app.CoalesceSetter,
	level *slog.LevelVar,
) {
	huma.Register(api, huma.Operation{
		OperationID: "listSettings", Method: "GET", Path: "/api/settings",
		Summary: "List runtime settings", Tags: []string{"settings"},
	}, getSettings(settings))

	// Registered before the {key} route below so "presets" is read as the literal
	// path segment it is rather than as a settings key.
	huma.Register(api, huma.Operation{
		OperationID: "listPresets", Method: "GET", Path: "/api/settings/presets",
		Summary:     "List settings presets",
		Description: "Named patches over the settings table, one per set of provider backends this build can serve.",
		Tags:        []string{"settings"},
	}, getPresets)

	huma.Register(api, huma.Operation{
		OperationID: "applyPreset", Method: "POST", Path: "/api/settings/presets/{name}/apply",
		Summary: "Apply a settings preset",
		//nolint:lll // one description, one line
		Description: "Writes every row the preset names, after validating all of them; returns only the rows that changed. Applies to the next task, so a video already running finishes on whichever backend each of its remaining tasks resolves at dispatch.",
		Tags:        []string{"settings"},
	}, applyPreset(settings, pools, coalesce, level))

	huma.Register(api, huma.Operation{
		OperationID: "updateSetting", Method: "PUT", Path: "/api/settings/{key}",
		Summary:     "Update a runtime setting",
		Description: "Applies immediately; pool limits, the SSE window and the log level need no restart.",
		Tags:        []string{"settings"},
	}, putSetting(settings, pools, coalesce, level))
}

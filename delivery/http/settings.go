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

	huma.Register(api, huma.Operation{
		OperationID: "updateSetting", Method: "PUT", Path: "/api/settings/{key}",
		Summary:     "Update a runtime setting",
		Description: "Applies immediately; pool limits, the SSE window and the log level need no restart.",
		Tags:        []string{"settings"},
	}, putSetting(settings, pools, coalesce, level))
}

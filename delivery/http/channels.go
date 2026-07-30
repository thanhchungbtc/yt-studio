package http

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// ChannelsOutput is the channel list response.
type ChannelsOutput struct {
	Body struct {
		Channels []ChannelDTO `json:"channels"`
	}
}

// ChannelOutput is a single channel response.
type ChannelOutput struct {
	Body ChannelDTO
}

// ChannelKeyInput resolves a channel by slug or id.
type ChannelKeyInput struct {
	Key string `path:"key" doc:"Channel slug or id"`
}

// CreateChannelInput is the create-channel request body.
type CreateChannelInput struct {
	Body struct {
		Slug        string        `json:"slug,omitempty" doc:"Lowercase kebab-case; derived from the name when omitted" maxLength:"64"`
		Name        string        `json:"name" required:"true" minLength:"1" maxLength:"120"`
		Description string        `json:"description,omitempty" maxLength:"1000"`
		Style       StyleInputDTO `json:"style,omitempty"`
	}
}

// UpdateChannelInput is the update-channel request. Every field is optional: a
// blank one leaves the stored value alone.
type UpdateChannelInput struct {
	Key  string `path:"key" doc:"Channel slug or id"`
	Body struct {
		Name        string        `json:"name,omitempty" maxLength:"120"`
		Description string        `json:"description,omitempty" maxLength:"1000"`
		Style       StyleInputDTO `json:"style,omitempty"`
		Credentials string        `json:"credentials,omitempty" enum:"missing,valid,expired"`
	}
}

// getChannels lists channels.
func getChannels(channels repository.ChannelReader) func(context.Context, *struct{}) (*ChannelsOutput, error) {
	return func(ctx context.Context, _ *struct{}) (*ChannelsOutput, error) {
		rows, err := app.ListChannels(ctx, channels)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ChannelsOutput{}
		out.Body.Channels = make([]ChannelDTO, 0, len(rows))
		for _, c := range rows {
			out.Body.Channels = append(out.Body.Channels, channelFrom(c))
		}
		return out, nil
	}
}

// getChannel resolves one channel by either key.
func getChannel(channels repository.ChannelReader) func(context.Context, *ChannelKeyInput) (*ChannelOutput, error) {
	return func(ctx context.Context, in *ChannelKeyInput) (*ChannelOutput, error) {
		c, err := app.GetChannel(ctx, channels, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		return &ChannelOutput{Body: channelFrom(c)}, nil
	}
}

// postChannel creates a channel.
func postChannel(writer repository.ChannelWriter, newID func() string, now func() time.Time) func(context.Context, *CreateChannelInput) (*ChannelOutput, error) {
	return func(ctx context.Context, in *CreateChannelInput) (*ChannelOutput, error) {
		c, err := app.CreateChannel(ctx, writer, newID, now(), app.CreateChannelInput{
			Slug:        in.Body.Slug,
			Name:        in.Body.Name,
			Description: in.Body.Description,
			Style:       in.Body.Style.Into(),
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &ChannelOutput{Body: channelFrom(c)}, nil
	}
}

// putChannel updates a channel's editable fields.
func putChannel(channels repository.ChannelReader, writer repository.ChannelWriter, now func() time.Time) func(context.Context, *UpdateChannelInput) (*ChannelOutput, error) {
	return func(ctx context.Context, in *UpdateChannelInput) (*ChannelOutput, error) {
		c, err := app.UpdateChannel(ctx, channels, writer, now(), in.Key, app.UpdateChannelInput{
			Name:        in.Body.Name,
			Description: in.Body.Description,
			Style:       in.Body.Style.Into(),
			Credentials: entity.CredentialStatus(in.Body.Credentials),
		})
		if err != nil {
			return nil, mapError(err)
		}
		return &ChannelOutput{Body: channelFrom(c)}, nil
	}
}

// deleteChannel removes a channel and everything it owns.
func deleteChannel(
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	videos repository.VideoReader,
	tasks repository.TaskWriter,
	forgetter app.VideoForgetter,
) func(context.Context, *ChannelKeyInput) (*struct{}, error) {
	return func(ctx context.Context, in *ChannelKeyInput) (*struct{}, error) {
		if err := app.DeleteChannel(ctx, channels, writer, videos, tasks, forgetter, in.Key); err != nil {
			return nil, mapError(err)
		}
		return nil, nil //nolint:nilnil // huma models 204 as a nil body
	}
}

func registerChannelRoutes(
	api huma.API,
	channels repository.ChannelReader,
	channelWriter repository.ChannelWriter,
	videos repository.VideoReader,
	tasks repository.TaskWriter,
	forgetter app.VideoForgetter,
	newID func() string,
	now func() time.Time,
) {
	huma.Register(api, huma.Operation{
		OperationID: "listChannels", Method: "GET", Path: "/api/channels",
		Summary: "List channels", Tags: []string{"channels"},
	}, getChannels(channels))

	huma.Register(api, huma.Operation{
		OperationID: "getChannel", Method: "GET", Path: "/api/channels/{key}",
		Summary: "Get a channel by slug or id", Tags: []string{"channels"},
	}, getChannel(channels))

	huma.Register(api, huma.Operation{
		OperationID: "createChannel", Method: "POST", Path: "/api/channels",
		Summary: "Create a channel", Tags: []string{"channels"}, DefaultStatus: 201,
	}, postChannel(channelWriter, newID, now))

	huma.Register(api, huma.Operation{
		OperationID: "updateChannel", Method: "PUT", Path: "/api/channels/{key}",
		Summary: "Update a channel", Tags: []string{"channels"},
	}, putChannel(channels, channelWriter, now))

	huma.Register(api, huma.Operation{
		OperationID: "deleteChannel", Method: "DELETE", Path: "/api/channels/{key}",
		Summary: "Delete a channel", Tags: []string{"channels"}, DefaultStatus: 204,
	}, deleteChannel(channels, channelWriter, videos, tasks, forgetter))
}

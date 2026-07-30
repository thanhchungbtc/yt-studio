package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// immutableCacheControl is safe because the URL contains the content address:
// the hash is the cache key, so a still caches forever for free.
const immutableCacheControl = "public, max-age=31536000, immutable"

// assetHandler streams a stored artifact.
//
// http.ServeContent is used rather than io.Copy so range requests work: the
// operator can scrub a three-hour render without the server buffering it.
func assetHandler(assets repository.AssetReader, store provider.AssetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := entity.AssetID(chi.URLParam(r, "id"))
		opened, err := app.OpenAsset(r.Context(), assets, store, id)
		if err != nil {
			writeAssetError(w, err)
			return
		}
		defer func() { _ = opened.Reader.Close() }()

		w.Header().Set("Content-Type", opened.Asset.MIME)
		w.Header().Set("Cache-Control", immutableCacheControl)
		w.Header().Set("ETag", `"`+string(opened.Asset.ID)+`"`)
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, string(opened.Asset.ID)+opened.Asset.Kind.Ext(), opened.Asset.CreatedAt, opened.Reader)
	}
}

func writeAssetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, entity.ErrAssetNotFound):
		http.Error(w, "asset not found", http.StatusNotFound)
	default:
		http.Error(w, "failed to read asset", http.StatusInternalServerError)
	}
}

// AssetsOutput lists a video's artifacts.
type AssetsOutput struct {
	Body struct {
		Assets []AssetDTO `json:"assets"`
	}
}

func getVideoAssets(videos repository.VideoReader, assets repository.AssetReader) func(context.Context, *VideoKeyInput) (*AssetsOutput, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*AssetsOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		rows, err := app.ListAssets(ctx, assets, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &AssetsOutput{}
		out.Body.Assets = make([]AssetDTO, 0, len(rows))
		for _, a := range rows {
			out.Body.Assets = append(out.Body.Assets, assetFrom(a))
		}
		return out, nil
	}
}

func registerAssetRoutes(api huma.API, videos repository.VideoReader, assets repository.AssetReader) {
	huma.Register(api, huma.Operation{
		OperationID: "listVideoAssets", Method: "GET", Path: "/api/videos/{key}/assets",
		Summary: "List a video's artifacts", Tags: []string{"assets"},
	}, getVideoAssets(videos, assets))
}

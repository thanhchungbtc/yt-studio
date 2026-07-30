package ninerouter

import (
	"github.com/tbui/yt-studio/domain/provider"
)

type NinerRouter struct{}

var _ provider.LLMProvider = (*NinerRouter)(nil)

func New() *NinerRouter {
	return &NinerRouter{}
}

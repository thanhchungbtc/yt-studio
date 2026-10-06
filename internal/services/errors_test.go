package services

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

func TestErrorsReachTheUIWithTheirKind(t *testing.T) {
	for err, want := range map[error]string{
		app.Invalid("title", "is required"):                   KindInvalid,
		fmt.Errorf("video SML-9: %w", repository.ErrNotFound): KindNotFound,
		fmt.Errorf("busy: %w", app.ErrConflict):               KindConflict,
		fmt.Errorf("disk on fire"):                            KindInternal,
	} {
		var p Problem
		if e := json.Unmarshal(MarshalError(err), &p); e != nil || p.Kind != want {
			t.Errorf("%v: kind %q, want %q", err, p.Kind, want)
		}
	}
}

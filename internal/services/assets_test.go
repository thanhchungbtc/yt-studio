package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestResourcesAreServedAndConfined(t *testing.T) {
	resources := fstest.MapFS{
		"background.jpg":  {Data: []byte("jpeg")},
		"fonts/Cabin.ttf": {Data: []byte("font")},
		"notes.txt":       {Data: []byte("not servable")},
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ui")) })
	h := AssetMiddleware(nil, nil, resources)(next)

	for _, tc := range []struct {
		path, body, mime string
		status           int
	}{
		{"/resources/background.jpg", "jpeg", "image/jpeg", 200},
		{"/resources/fonts/Cabin.ttf", "font", "font/ttf", 200},
		{"/resources/notes.txt", "", "", 404},
		{"/resources/../secrets.jpg", "", "", 404},
		{"/resources/missing.png", "", "", 404},
		{"/index.html", "ui", "", 200},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.path, rec.Code, tc.status)
			continue
		}
		if tc.status == 200 && rec.Body.String() != tc.body {
			t.Errorf("%s: body %q", tc.path, rec.Body.String())
		}
		if tc.mime != "" && rec.Header().Get("Content-Type") != tc.mime {
			t.Errorf("%s: type %q", tc.path, rec.Header().Get("Content-Type"))
		}
	}
}

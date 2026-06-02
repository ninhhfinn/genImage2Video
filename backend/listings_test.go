package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSelectListing_SlowImageDoesNotHang reproduces the batch-render failure: a
// single hung/slow photo URL must not stall the handler past the client
// timeout. With the per-request cap, the handler returns promptly, counts the
// good photos, and reports the slow + non-200 ones as errors.
func TestSelectListing_SlowImageDoesNotHang(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/slow"):
			time.Sleep(3 * time.Second) // longer than the shortened client timeout
			w.Write([]byte("late"))
		case strings.HasPrefix(r.URL.Path, "/notfound"):
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("<html>nope</html>"))
		default:
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("\xff\xd8\xff\xe0jpegish"))
		}
	}))
	defer srv.Close()

	// Shorten the per-request timeout for the test; restore after.
	orig := imageDownloadClient.Timeout
	imageDownloadClient.Timeout = 250 * time.Millisecond
	defer func() { imageDownloadClient.Timeout = orig }()

	body := fmt.Sprintf(`{"photo_urls":["%s/good1.jpg","%s/slow.jpg","%s/notfound.jpg","%s/good2.jpg"]}`,
		srv.URL, srv.URL, srv.URL, srv.URL)

	h := selectListingHandler(t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/api/select-listing", strings.NewReader(body))
	rec := httptest.NewRecorder()

	start := time.Now()
	h(rec, req)
	elapsed := time.Since(start)

	// Must return well before the 3s slow sleep (per-request cap is 250ms).
	if elapsed > 2*time.Second {
		t.Fatalf("handler hung %v — per-request timeout not applied", elapsed)
	}

	var out struct {
		Count  int      `json:"count"`
		Total  int      `json:"total"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode resp: %v (%s)", err, rec.Body.String())
	}
	if out.Total != 4 {
		t.Errorf("total = %d, want 4", out.Total)
	}
	if out.Count != 2 {
		t.Errorf("count = %d, want 2 (two good photos)", out.Count)
	}
	if len(out.Errors) != 2 {
		t.Errorf("errors = %v, want 2 (slow + 404)", out.Errors)
	}
}

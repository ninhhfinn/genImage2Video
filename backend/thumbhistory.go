package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Thumbnail history — the batch-render counterpart to render history (excel.go).
// When many listings are rendered, each one's collage thumbnail is persisted
// here so they can be reviewed/downloaded together.

type ThumbnailRecord struct {
	ListingName string `json:"listing_name"`
	ListingID   string `json:"listing_id"`
	FileName    string `json:"filename"`
	RenderTime  string `json:"render_time"`
}

var (
	thumbnailHistory []ThumbnailRecord
	thumbHistoryMu   sync.Mutex
)

func addThumbnailToHistory(rec ThumbnailRecord) {
	thumbHistoryMu.Lock()
	defer thumbHistoryMu.Unlock()
	thumbnailHistory = append(thumbnailHistory, rec)
}

// saveThumbnailFile writes the JPEG bytes under a unique name. Millisecond
// precision + the listing id keep batch renders from colliding.
func saveThumbnailFile(data []byte, outputDir, listingID string) (string, error) {
	ts := time.Now().Format("20060102_150405.000")
	name := fmt.Sprintf("thumb_%s.jpg", ts)
	if listingID != "" {
		name = fmt.Sprintf("thumb_%s_%s.jpg", sanitizeFilename(listingID), ts)
	}
	if err := os.WriteFile(filepath.Join(outputDir, name), data, 0644); err != nil {
		return "", err
	}
	return name, nil
}

func thumbnailHistoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		thumbHistoryMu.Lock()
		records := make([]ThumbnailRecord, len(thumbnailHistory))
		copy(records, thumbnailHistory)
		thumbHistoryMu.Unlock()

		type row struct {
			ThumbnailRecord
			URL string `json:"url"` // relative — same origin as the web app
		}
		rows := make([]row, len(records))
		for i, rec := range records {
			rows[i] = row{rec, "/api/thumbnail-file?name=" + rec.FileName}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": rows,
			"count":   len(rows),
		})
	}
}

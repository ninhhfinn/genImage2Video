package main

// Render 2 listing CỤ THỂ trong screenshot user gửi (data API thật từ
// testdata/listings_fresh.json) × 8 template, build Config y hệt production
// (App.jsx buildRenderCfg). Soi bug template với data thật.
//   GENSHOT=1 go test -count=1 -run TestGenScreenshotListings .

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
)

func TestGenScreenshotListings(t *testing.T) {
	if os.Getenv("GENSHOT") == "" {
		t.Skip("set GENSHOT=1")
	}
	const W, H = 1080, 1920
	data, err := os.ReadFile("testdata/listings_fresh.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	byID := map[string]ListingInfo{}
	for _, in := range infos {
		byID[in.ID] = in
	}
	wantIDs := []string{"lsv3ix6oij", "ls2pw65344"}
	photoDir := "testdata/realphotos"
	os.MkdirAll(photoDir, 0o755)
	outDir := "../shot-out"
	os.MkdirAll(outDir, 0o755)
	templates := []string{"goldserif", "creampill", "staycation", "chillgreen", "marquee", "editorial", "amorex", "ntgroom"}

	for _, id := range wantIDs {
		in, ok := byID[id]
		if !ok {
			t.Fatalf("listing %s không có trong data", id)
		}
		bg := refV1
		if len(in.ThumbURLs) > 0 {
			dst := filepath.Join(photoDir, in.ID+"-0.jpg")
			if p, derr := downloadPhoto(in.ThumbURLs[0], dst); derr == nil {
				bg = p
			}
		}
		nick := in.Nickname
		if nick == "" {
			nick = in.Name
		}
		amen := strings.Join(curateAmenitiesVI(in.Amenities), ", ")
		t.Logf("=== %s | nick=%q addr=%q\n  prices=%v", id, nick, in.Address, in.PriceLines)
		for _, tmpl := range templates {
			cfg := Config{
				Template: tmpl, Width: W, Height: H,
				Nickname:  nick,
				Address:   in.Address,
				Prices:    in.PriceLines,
				ListingID: in.ID, // production gửi id — kiểm tra KHÔNG còn đè
			}
			if amen != "" {
				cfg.Amenities = []string{amen}
			}
			if tmpl == "editorial" {
				cfg.Watermark = "Dayladau House"
			}
			dc := gg.NewContext(W, H)
			if img, e := imaging.Open(bg, imaging.AutoOrientation(true)); e == nil {
				dc.DrawImage(imaging.Fill(img, W, H, imaging.Center, imaging.Lanczos), 0, 0)
			} else {
				dc.SetColor(color.NRGBA{40, 30, 26, 255})
				dc.DrawRectangle(0, 0, W, H)
				dc.Fill()
			}
			tmp := t.TempDir()
			plans, ok, perr := renderVideoTemplateComposite(cfg, tmp)
			if perr != nil || !ok {
				t.Fatalf("%s-%s: ok=%v err=%v", id, tmpl, ok, perr)
			}
			for _, p := range plans {
				if ov, e := imaging.Open(p.PNGPath); e == nil {
					dc.DrawImage(ov, p.X, p.Y)
				}
			}
			safe := strings.NewReplacer("/", "_", " ", "_").Replace(nick)
			if len(safe) > 16 {
				safe = safe[:16]
			}
			out := filepath.Join(outDir, id+"-"+safe+"--"+tmpl+".jpg")
			imaging.Save(dc.Image(), out, imaging.JPEGQuality(88))
		}
	}
}

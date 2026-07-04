package main

// End-to-end MP4 THẬT từ data API: parseListings(testdata) → tải nhiều ảnh thật →
// buildKenBurns + runFFmpeg (CHÍNH đường /api/render) cho cả 8 template → trích frame.
// Chứng minh pipeline đầy đủ (ken burns + chuyển cảnh + overlay composite) ra video
// đúng giao diện với data thật.
//   GENAPIVID=1 go test -count=1 -timeout 900s -run TestGenRealAPIVideo .

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenRealAPIVideo(t *testing.T) {
	if os.Getenv("GENAPIVID") == "" {
		t.Skip("set GENAPIVID=1")
	}
	const W, H = 1080, 1920
	data, err := os.ReadFile("testdata/listings.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	if len(infos) == 0 {
		t.Fatal("no listings")
	}
	photoDir := "testdata/realphotos"
	os.MkdirAll(photoDir, 0o755)
	outDir := "../realapi-video-out"
	os.MkdirAll(outDir, 0o755)

	templates := []string{"goldserif", "creampill", "staycation", "chillgreen", "marquee", "editorial", "amorex", "ntgroom"}
	// 2 listing: 501 (data đầy đủ + combo dài), 502 (combo 147 ký tự — ca tệ nhất).
	for _, idx := range []int{6, 15} {
		if idx >= len(infos) {
			continue
		}
		in := infos[idx]

		// Tải tối đa 3 ảnh THẬT (full-res) → ken burns + chuyển cảnh.
		var imgs []string
		for i := 0; i < len(in.PhotoURLs) && len(imgs) < 3; i++ {
			dst := filepath.Join(photoDir, in.ID+"-full-"+string(rune('a'+i))+".jpg")
			if p, derr := downloadPhoto(in.PhotoURLs[i], dst); derr == nil {
				imgs = append(imgs, p)
			}
		}
		if len(imgs) == 0 {
			imgs = []string{refV1}
		}
		nick := in.Nickname
		if nick == "" {
			nick = in.Name
		}
		amen := curateAmenitiesVI(in.Amenities)
		safe := strings.NewReplacer("/", "_", " ", "_").Replace(nick)
		if len(safe) > 14 {
			safe = safe[:14]
		}

		for _, tmpl := range templates {
			cfg := Config{
				Template: tmpl, Mode: "kenburns",
				Width: W, Height: H, Tiktok: true, FPS: 30,
				ZoomIntensity: 0.5, Total: 4.5,
				Nickname: nick, Address: in.Address, Prices: in.PriceLines,
			}
			if line := strings.Join(amen, ", "); line != "" {
				cfg.Amenities = []string{line}
			}
			if tmpl == "editorial" {
				cfg.Watermark = "Dayladau House"
			}
			cfg.Duration = cfg.Total / float64(len(imgs))
			cfg.Output = filepath.Join(outDir, safe+"--"+tmpl+".mp4")

			args, berr := buildKenBurns(cfg, imgs)
			if berr != nil {
				t.Fatalf("%s/%s build: %v", safe, tmpl, berr)
			}
			if rerr := runFFmpeg(args, cfg); rerr != nil {
				t.Fatalf("%s/%s ffmpeg: %v", safe, tmpl, rerr)
			}
			frame := filepath.Join(outDir, safe+"--"+tmpl+".png")
			ex := exec.Command("ffmpeg", "-y", "-ss", "2.5", "-i", cfg.Output, "-frames:v", "1", frame)
			if out, eerr := ex.CombinedOutput(); eerr != nil {
				t.Fatalf("%s/%s frame: %v\n%s", safe, tmpl, eerr, out)
			}
		}
		t.Logf("✓ #%d %s (%d real photos) -> 8 MP4", idx, nick, len(imgs))
	}
}

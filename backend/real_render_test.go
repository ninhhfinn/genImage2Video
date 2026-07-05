package main

// Real end-to-end render: gọi CHÍNH đường production buildKenBurns + runFFmpeg
// (giống hệt handler /api/render) để xuất MP4 thật, rồi trích 1 frame ra PNG so
// với mockup. Run: GENREAL=1 go test -count=1 -run TestGenRealVideo .

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGenRealVideo(t *testing.T) {
	if os.Getenv("GENREAL") == "" {
		t.Skip("set GENREAL=1")
	}
	const W, H = 1080, 1920
	outDir := "../real-render-out"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Ảnh nền thật (nhiều ảnh → ken burns + chuyển cảnh như video thật).
	var imgs []string
	for _, n := range []string{"room01.jpg", "room03.jpg", "room05.jpg"} {
		p := filepath.Join("../template-mockups/_rooms", n)
		if _, err := os.Stat(p); err == nil {
			imgs = append(imgs, p)
		}
	}
	if len(imgs) == 0 {
		imgs = []string{refV1}
	}

	cases := []struct {
		name string
		cfg  Config
	}{
		{"real-1-goldserif", Config{Template: "goldserif", Nickname: "Amoureux NK605",
			Address: "Đường Nguyễn Khang, Cầu Giấy",
			Prices:  []string{"Giá theo ngày\n2N1Đ: 390,000\nQua đêm: 290,000", "Giá theo giờ\n14h - 20h: 390,000\n14h - 18h: 290,000"}}},
		{"real-2-creampill", Config{Template: "creampill", Nickname: "Amoureux NK605",
			Address: "Đường Nguyễn Khang, Cầu Giấy",
			Prices:  []string{"Qua đêm: 299,000", "Combo: 299,000"}}},
		{"real-3-staycation", Config{Template: "staycation", Nickname: "Amoureux NK605",
			Address: "Đường Nguyễn Khang, Cầu Giấy", Amenities: []string{"Máy chiếu Netflix, bồn tắm, boardgame"},
			Prices: []string{"2N1Đ: 299,000\nQua đêm: 299,000\nCombo theo giờ: 299,000"}}},
		{"real-4-chillgreen", Config{Template: "chillgreen",
			Nickname: "Homestay chữa lành siu chill\nđầy đủ tiện nghi, máy chiếu Netflix",
			Address:  "Thanh Xuân - Hà Nội",
			Prices:   []string{"21h - 9h (Qua đêm): 279\n2N1Đ: 399", "10h - 13h: 299\n10h - 20h: 299\n14h - 20h: 299"}}},
		{"real-5-marquee", Config{Template: "marquee",
			Amenities: []string{"Máy chiếu Netflix – Bếp cooking date"},
			Prices:    []string{"21h - 9h (Qua đêm): 314\n2N1Đ: 449", "10h - 13h: 199\n10h - 20h: 299\n14h - 20h: 279"}}},
		{"real-6-editorial", Config{Template: "editorial", Nickname: "Standard",
			Address: "số 4 ngõ 444 Đội Cấn, Ba Đình", Watermark: "Dayladau House",
			Prices:  []string{"Giá giờ: 349K/2H\nGiá đêm/ngày: 589K\nGiá ngày/đêm: 689K"}}},
		{"real-7-amorex", Config{Template: "amorex", Nickname: "Amourex",
			Address: "Vũ Tông Phan, Thanh Xuân, Hà Nội",
			Prices:  []string{"Giá giờ: 299 cá\nGiá qua đêm: 299 cá\n2N1Đ: 299 cá"}}},
		{"real-8-ntgroom", Config{Template: "ntgroom", Nickname: "NTG402",
			Address: "Vũ Tông Phan, Thanh Xuân, Hà Nội",
			Prices:  []string{"2N1Đ: 399k\nQua đêm: 299 k\nThêm giờ: 80k\n2h đầu: 199k\nCombo giờ: 299k"}}},
	}

	for _, c := range cases {
		cfg := c.cfg
		cfg.Mode = "kenburns"
		cfg.Width, cfg.Height = W, H
		cfg.Tiktok = true
		cfg.FPS = 30
		cfg.ZoomIntensity = 0.5
		cfg.Total = 5
		cfg.Duration = cfg.Total / float64(len(imgs))
		cfg.Output = filepath.Join(outDir, c.name+".mp4")

		args, err := buildKenBurns(cfg, imgs)
		if err != nil {
			t.Fatalf("%s build: %v", c.name, err)
		}
		if err := runFFmpeg(args, cfg); err != nil {
			t.Fatalf("%s ffmpeg: %v", c.name, err)
		}
		// Trích 1 frame ở t=3.0s (overlay luôn hiện với template Go-code).
		frame := filepath.Join(outDir, c.name+".png")
		ex := exec.Command("ffmpeg", "-y", "-ss", "3.0", "-i", cfg.Output, "-frames:v", "1", frame)
		if out, err := ex.CombinedOutput(); err != nil {
			t.Fatalf("%s extract frame: %v\n%s", c.name, err, out)
		}
		t.Logf("✓ %s -> %s + frame %s", c.name, cfg.Output, frame)
	}
}

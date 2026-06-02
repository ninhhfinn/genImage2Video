package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"img2video/textrender"
)

// OverlayPlan describes one PNG that ffmpeg should overlay onto the video.
type OverlayPlan struct {
	PNGPath    string
	X, Y       int
	EnableExpr string // ffmpeg overlay enable expression; "" = always on
}

// prepareTextOverlayPlans is the entry called by build*() in main.go.
// Creates a temp dir, calls renderTextOverlays, returns (plans, tmpDir, err).
// Caller is responsible for `defer os.RemoveAll(tmpDir)`.
// Returns nil plans + empty tmpDir if no template is configured (caller falls back).
func prepareTextOverlayPlans(cfg Config) ([]OverlayPlan, string, error) {
	if strings.TrimSpace(cfg.Template) == "" {
		return nil, "", nil
	}
	tmpDir, err := os.MkdirTemp("", "img2video_textovl_*")
	if err != nil {
		return nil, "", err
	}
	plans, err := renderTextOverlays(cfg, tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", err
	}
	return plans, tmpDir, nil
}

// renderTextOverlays loads cfg.Template, paints each element via the textrender
// package, writes the resulting PNGs into tmpDir, and returns the overlay plan.
// Returns nil, nil when no template is selected — the caller should fall back
// to the legacy magick-based renderListingOverlayPNG path.
func renderTextOverlays(cfg Config, tmpDir string) ([]OverlayPlan, error) {
	tmplName := strings.TrimSpace(strings.ToLower(cfg.Template))
	if tmplName == "" {
		return nil, nil
	}
	tmpl, err := textrender.LoadTemplate(tmplName, assetsTemplatesDir())
	if err != nil {
		return nil, err
	}
	if len(cfg.CustomStyles) > 0 {
		tmpl = textrender.Merge(tmpl, cfg.CustomStyles)
	}
	applyConfigOverrides(tmpl, cfg)

	ctx := &textrender.RenderContext{
		VideoWidth:  cfg.Width,
		VideoHeight: cfg.Height,
		AssetsDir:   assetsDir(),
	}

	titleEnable, listingEnable := "", ""
	if strings.TrimSpace(cfg.Title) != "" {
		td := cfg.TitleDuration
		if td <= 0 {
			td = 3
		}
		titleEnable = fmt.Sprintf("between(t,0,%.3f)", td)
		listingEnable = fmt.Sprintf("gte(t,%.3f)", td)
	}

	var plans []OverlayPlan
	add := func(key, text, enable string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		st, ok := tmpl.Elements[key]
		if !ok || st == nil {
			return
		}
		s := st.Clone()
		s.Text = text
		adaptFont(s)
		out, err := textrender.Render(s, ctx)
		if err != nil || out == nil {
			return
		}
		path := filepath.Join(tmpDir, key+".png")
		if err := out.Save(path); err != nil {
			return
		}
		plans = append(plans, OverlayPlan{PNGPath: path, X: out.X, Y: out.Y, EnableExpr: enable})
	}

	add("title", cfg.Title, titleEnable)

	// Listing-info cluster: lay out as a vertical flow stack when the template
	// defines one, so multi-line text never overlaps. Otherwise fall back to
	// each element's own fixed position.
	inStack := map[string]bool{}
	if tmpl.Stack != nil && len(tmpl.Stack.Items) > 0 {
		for _, k := range tmpl.Stack.Items {
			inStack[k] = true
		}
		plans = append(plans, renderStack(tmpl, ctx, cfg, tmpDir, listingEnable)...)
	}
	if !inStack["address"] {
		add("address", cfg.Address, listingEnable)
	}
	if !inStack["nickname"] {
		add("nickname", strings.TrimPrefix(cfg.Nickname, "Homestay "), listingEnable)
	}
	if !inStack["prices"] {
		if joined := joinPricesForOverlay(cfg.Prices); joined != "" {
			add("prices", joined, listingEnable)
		}
	}
	if !inStack["amenities"] {
		if joined := joinAmenitiesForOverlay(cfg.Amenities); joined != "" {
			add("amenities", joined, listingEnable)
		}
	}
	add("listing_id", cfg.ListingID, listingEnable)
	add("watermark", cfg.Watermark, "")

	return plans, nil
}

// listingText resolves the display text for a stack item key.
func listingText(key string, cfg Config) string {
	switch key {
	case "address":
		return strings.TrimSpace(cfg.Address)
	case "nickname":
		return strings.TrimSpace(strings.TrimPrefix(cfg.Nickname, "Homestay "))
	case "prices":
		return joinPricesForOverlay(cfg.Prices)
	case "amenities":
		return joinAmenitiesForOverlay(cfg.Amenities)
	case "title":
		return strings.TrimSpace(cfg.Title)
	case "listing_id":
		return strings.TrimSpace(cfg.ListingID)
	case "watermark":
		return strings.TrimSpace(cfg.Watermark)
	}
	return ""
}

// adaptFont swaps Yeseva One → Playfair when the text has non-ASCII (Vietnamese)
// characters, since Yeseva One lacks Vietnamese diacritic glyphs. The curve/style
// is unaffected — only the font file changes.
func adaptFont(s *textrender.ElementStyle) {
	if s.FontFile != "YesevaOne-Regular.ttf" {
		return
	}
	for _, r := range s.Text {
		if r > 127 {
			s.FontFile = "PlayfairDisplay-Variable.ttf"
			return
		}
	}
}

// renderStack renders the template's stack items in order, measures each, and
// places them sequentially with a fixed margin so they never overlap. The whole
// group is anchored per Stack.Anchor around Stack.Y.
func renderStack(tmpl *textrender.Template, ctx *textrender.RenderContext, cfg Config, tmpDir, enable string) []OverlayPlan {
	st := tmpl.Stack
	margin := int(st.Margin)

	type item struct {
		key string
		el  *textrender.RenderedElement
	}
	var items []item
	totalH := 0
	for _, key := range st.Items {
		text := listingText(key, cfg)
		if text == "" {
			continue
		}
		style := tmpl.Elements[key]
		if style == nil {
			continue
		}
		s := style.Clone()
		s.Text = text
		adaptFont(s)
		out, err := textrender.Render(s, ctx)
		if err != nil || out == nil {
			continue
		}
		items = append(items, item{key, out})
	}
	if len(items) == 0 {
		return nil
	}
	for i, it := range items {
		totalH += it.el.Height
		if i < len(items)-1 {
			totalH += margin
		}
	}

	yRef := resolveYRef(st.Y, ctx.VideoHeight)
	var top int
	switch strings.ToLower(strings.TrimSpace(st.Anchor)) {
	case "top":
		top = yRef
	case "bottom":
		top = yRef - totalH
	default: // center
		top = yRef - totalH/2
	}

	// First pass: position each item top→bottom from the group anchor.
	type placed struct {
		key  string
		el   *textrender.RenderedElement
		x, y int
	}
	var ps []placed
	cur := top
	for _, it := range items {
		x := resolveStackX(st.X, it.el.Width, ctx.VideoWidth)
		ps = append(ps, placed{it.key, it.el, x, cur})
		cur += it.el.Height + margin
	}

	var plans []OverlayPlan

	// Scrim panel: hug only the body items (skip title/nickname so the title sits
	// ABOVE the band, not inside it), left-aligned to the content.
	if st.Scrim != nil {
		sLeft, sRight, sTop, sBottom := 1<<30, -(1 << 30), 1<<30, -(1 << 30)
		found := false
		for _, p := range ps {
			if p.key == "nickname" || p.key == "title" {
				continue
			}
			found = true
			if p.x < sLeft {
				sLeft = p.x
			}
			if p.x+p.el.Width > sRight {
				sRight = p.x + p.el.Width
			}
			if p.y < sTop {
				sTop = p.y
			}
			if p.y+p.el.Height > sBottom {
				sBottom = p.y + p.el.Height
			}
		}
		if found {
			padV := int(st.Scrim.Padding[0])
			padH := int(st.Scrim.Padding[1])
			sx := sLeft - padH
			sy := sTop - padV
			sw := (sRight - sLeft) + padH*2
			sh := (sBottom - sTop) + padV*2
			if sw > 0 && sh > 0 {
				if pngBytes, err := textrender.RenderPanel(sw, sh, st.Scrim.Color, st.Scrim.Alpha, st.Scrim.Radius); err == nil {
					path := filepath.Join(tmpDir, "scrim.png")
					if os.WriteFile(path, pngBytes, 0644) == nil {
						plans = append(plans, OverlayPlan{PNGPath: path, X: sx, Y: sy, EnableExpr: enable})
					}
				}
			}
		}
	}

	for _, p := range ps {
		path := filepath.Join(tmpDir, p.key+".png")
		if err := p.el.Save(path); err != nil {
			continue
		}
		plans = append(plans, OverlayPlan{PNGPath: path, X: p.x, Y: p.y, EnableExpr: enable})
	}
	return plans
}

// resolveYRef converts a stack Y spec (fraction of height or "Npx") to pixels.
func resolveYRef(spec string, canvasH int) int {
	s := strings.TrimSpace(spec)
	if s == "" {
		return canvasH / 2
	}
	if strings.HasSuffix(s, "px") {
		n, _ := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
		return int(n)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f * float64(canvasH))
	}
	return canvasH / 2
}

// resolveStackX converts a stack X spec to the element's left pixel coordinate.
func resolveStackX(spec string, elemW, canvasW int) int {
	s := strings.TrimSpace(spec)
	switch strings.ToLower(s) {
	case "", "center", "middle":
		return (canvasW - elemW) / 2
	case "left":
		return 0
	case "right":
		return canvasW - elemW
	}
	if strings.HasSuffix(s, "px") {
		n, _ := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
		return int(n)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f * float64(canvasW))
	}
	return (canvasW - elemW) / 2
}

func joinPricesForOverlay(prices []string) string {
	var clean []string
	for _, p := range prices {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	if len(clean) <= 3 {
		return strings.Join(clean, " · ")
	}
	// 4+ prices → split into 2 lines
	mid := (len(clean) + 1) / 2
	return strings.Join(clean[:mid], " · ") + "\n" + strings.Join(clean[mid:], " · ")
}

func joinAmenitiesForOverlay(amenities []string) string {
	var clean []string
	for _, a := range amenities {
		a = strings.TrimSpace(a)
		if a != "" {
			clean = append(clean, a)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	// Wrap into ~3 amenities per line, max 3 lines = 9 amenities shown
	const perLine = 3
	var lines []string
	for i := 0; i < len(clean) && len(lines) < 3; i += perLine {
		end := i + perLine
		if end > len(clean) {
			end = len(clean)
		}
		lines = append(lines, strings.Join(clean[i:end], " · "))
	}
	return strings.Join(lines, "\n")
}

// buildOverlayFilterChain returns the filter_complex segment that overlays each
// PNG in `plans` onto the input label `baseLabel` (e.g. "[vtrim]"), producing
// the named output label `outLabel` (e.g. "[vfinal]"). The PNG inputs are
// indexed starting at `firstInputIdx` in the ffmpeg argv.
func buildOverlayFilterChain(plans []OverlayPlan, baseLabel, outLabel string, firstInputIdx int) string {
	if len(plans) == 0 {
		return ""
	}
	var sb strings.Builder
	prev := baseLabel
	for i, p := range plans {
		enable := ""
		if p.EnableExpr != "" {
			enable = fmt.Sprintf(":enable='%s'", p.EnableExpr)
		}
		next := outLabel
		if i < len(plans)-1 {
			next = fmt.Sprintf("[ovl%d]", i)
		}
		sb.WriteString(fmt.Sprintf(";\n%s[%d:v]overlay=%d:%d%s%s",
			prev, firstInputIdx+i, p.X, p.Y, enable, next))
		prev = next
	}
	return sb.String()
}

// assetsDir resolves the path to backend/assets. Looks next to the binary first,
// then falls back to the source-tree location for dev runs.
func assetsDir() string {
	candidates := []string{"assets", "./assets"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "assets"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "assets"
}

func assetsTemplatesDir() string {
	return filepath.Join(assetsDir(), "templates")
}

// applyConfigOverrides patches a loaded template with user-facing knobs from the
// settings panel (font choice, scale, color, badge/bubble pill) so those
// controls actually affect render output. Without this bridge the template JSON
// fully dictates the look and the UI toggles are dead.
//
// Keys touched:
//   - "title"                                  : TextFont / TextScale / TextColor
//   - "address","nickname","prices","amenities","listing_id" :
//       OverlayFont / OverlayScale / OverlayText, plus OverlayStyle → Bg toggle,
//       OverlayBG → Bg.Color, OverlayStroke → Shadow.Color (bubble stroke proxy).
//   - "watermark"                              : color from TextColor
func applyConfigOverrides(tmpl *textrender.Template, cfg Config) {
	if tmpl == nil || tmpl.Elements == nil {
		return
	}

	bodyKeys := []string{"address", "prices", "amenities", "listing_id"}
	titleKeys := []string{"title", "nickname"}

	overlayFontFile := fontFileForChoice(cfg.OverlayFont)
	titleFontFile := strings.TrimSpace(cfg.TitleFontFile)
	overlayScale := scaleMultiplier(cfg.OverlayScale)
	bodyColor := strings.TrimSpace(cfg.OverlayText)
	bodyBg := strings.TrimSpace(cfg.BodyBg)
	titleColor := strings.TrimSpace(cfg.TitleColor)
	titleBg := strings.TrimSpace(cfg.TitleBg)
	strokeColor := strings.TrimSpace(cfg.StrokeColor)

	// Body (address + content): font / scale / text color / pill color. We only
	// re-color an existing pill — never add or strip one (style follows template).
	for _, key := range bodyKeys {
		st := tmpl.Elements[key]
		if st == nil {
			continue
		}
		if overlayFontFile != "" {
			st.FontFile = overlayFontFile
		}
		if overlayScale != 1 {
			st.SizePct = st.SizePct * overlayScale
		}
		if bodyColor != "" {
			st.Color = bodyColor
		}
		if st.Bg != nil && bodyBg != "" {
			st.Bg.Color = bodyBg
		}
	}

	// Title (and intro title): fill color / outline color / pill color, all
	// independent of the body so the two-tone look is preserved.
	for _, key := range titleKeys {
		st := tmpl.Elements[key]
		if st == nil {
			continue
		}
		if titleFontFile != "" {
			st.FontFile = titleFontFile
		}
		if titleColor != "" {
			st.Color = titleColor
		}
		if strokeColor != "" && st.Stroke != nil {
			st.Stroke.Color = strokeColor
		}
		if st.Bg != nil && titleBg != "" {
			st.Bg.Color = titleBg
		}
	}

	if wm := tmpl.Elements["watermark"]; wm != nil && bodyColor != "" {
		wm.Color = bodyColor
	}
}

// fontFileForChoice maps the UI font dropdown values ("playfair" / "lilita")
// to the actual TTF file names shipped under assets/fonts. Empty input → "".
func fontFileForChoice(choice string) string {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "lilita", "lilita-one":
		return "LilitaOne-Regular.ttf"
	case "playfair", "playfair-display":
		return "PlayfairDisplay-Variable.ttf"
	case "yeseva", "yeseva-one":
		return "YesevaOne-Regular.ttf"
	default:
		// Pass through a raw font filename (e.g. an uploaded "uploads/x.ttf" or a
		// shipped ".ttf"/".otf") so custom fonts work in the listing-font tool too.
		c := strings.TrimSpace(choice)
		if strings.HasSuffix(strings.ToLower(c), ".ttf") || strings.HasSuffix(strings.ToLower(c), ".otf") {
			return c
		}
		return ""
	}
}

// scaleMultiplier clamps the user-provided scale to the safe range used by the
// legacy magick path. 0 or unset → no scaling (returns 1).
func scaleMultiplier(v float64) float64 {
	if v <= 0 {
		return 1
	}
	if v < 0.6 {
		return 0.6
	}
	if v > 1.6 {
		return 1.6
	}
	return v
}

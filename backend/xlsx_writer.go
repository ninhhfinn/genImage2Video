package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// ─── Types ────────────────────────────────────────────────────────────────

type xlsxCell struct {
	value   string
	isNum   bool
	numVal  float64
	isLink  bool
	linkURL string
	styleID int
}

type xlsxRow struct {
	cells  []xlsxCell
	height float64
}

type hlEntry struct{ ref, url, rid string }

type xlsxSheet struct {
	rows        []xlsxRow
	colWidths   []float64
	freezeRow   int
	sharedStrs  []string
	strIndex    map[string]int
	hyperlinks  []hlEntry
}

func newSheet() *xlsxSheet {
	return &xlsxSheet{strIndex: make(map[string]int)}
}

func (s *xlsxSheet) intern(v string) int {
	if i, ok := s.strIndex[v]; ok {
		return i
	}
	i := len(s.sharedStrs)
	s.sharedStrs = append(s.sharedStrs, v)
	s.strIndex[v] = i
	return i
}

func (s *xlsxSheet) addRow(height float64, cells ...xlsxCell) {
	s.rows = append(s.rows, xlsxRow{cells: cells, height: height})
}

// ─── Cell constructors ────────────────────────────────────────────────────

func strCell(v string, style int) xlsxCell { return xlsxCell{value: v, styleID: style} }
func numCell(v float64, style int) xlsxCell { return xlsxCell{isNum: true, numVal: v, styleID: style} }
func linkCell(text, url string) xlsxCell    { return xlsxCell{value: text, isLink: true, linkURL: url, styleID: 3} }
func emptyCell(style int) xlsxCell          { return xlsxCell{styleID: style} }

// ─── Build Excel from history ─────────────────────────────────────────────

func buildExcelFromHistory(records []RenderRecord, baseURL string) ([]byte, error) {
	s := newSheet()
	s.colWidths = []float64{6, 28, 36, 9, 11, 13, 20, 56, 16}
	s.freezeRow = 3

	now := time.Now().Format("02/01/2006 15:04:05")

	// Row 1: tiêu đề
	r1 := []xlsxCell{strCell(fmt.Sprintf("img2video — Danh sách video  |  %s  |  Tổng: %d video", now, len(records)), 1)}
	for i := 1; i < 9; i++ {
		r1 = append(r1, emptyCell(1))
	}
	s.addRow(28, r1...)

	// Row 2: spacer
	spacer := make([]xlsxCell, 9)
	for i := range spacer { spacer[i] = emptyCell(0) }
	s.addRow(8, spacer...)

	// Row 3: header
	headers := []string{"STT", "Tên listing", "Địa chỉ", "Số ảnh", "Thời lượng", "Kích thước", "Ngày tạo", "URL tải xuống", "Ghi chú"}
	hrow := make([]xlsxCell, 9)
	for i, h := range headers {
		hrow[i] = strCell(h, 2)
	}
	s.addRow(22, hrow...)

	// Data rows
	totalMB := 0.0
	for i, rec := range records {
		dlURL := baseURL + "/api/video/" + rec.FileName
		totalMB += rec.FileSizeMB
		style := 6 // zebra
		if i%2 == 0 { style = 0 }

		ctr := style
		if style == 6 { ctr = 7 } else { ctr = 5 }

		row := []xlsxCell{
			numCell(float64(i+1), ctr),
			strCell(rec.ListingName, style),
			strCell(rec.Address, style),
			numCell(float64(rec.PhotoCount), ctr),
			strCell(fmt.Sprintf("%.0fs", rec.Duration), ctr),
			strCell(fmt.Sprintf("%.1f MB", rec.FileSizeMB), ctr),
			strCell(rec.RenderTime, style),
			linkCell(dlURL, dlURL),
			strCell(rec.Note, style),
		}
		s.addRow(20, row...)
	}

	// Total row
	total := make([]xlsxCell, 9)
	total[0] = strCell(fmt.Sprintf("Tổng: %d video", len(records)), 4)
	for i := 1; i < 6; i++ { total[i] = emptyCell(4) }
	total[6] = strCell(fmt.Sprintf("%.1f MB", totalMB), 4)
	total[7] = emptyCell(4)
	total[8] = emptyCell(4)
	s.addRow(22, total...)

	return writeXLSX(s)
}

// ─── XLSX writer ──────────────────────────────────────────────────────────

func writeXLSX(s *xlsxSheet) ([]byte, error) {
	// CRITICAL: generate sheet XML first — this populates s.sharedStrs & s.hyperlinks
	sheetContent     := buildSheetXML(s)
	sheetRelsContent := buildSheetRels(s)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil { return err }
		_, err = w.Write([]byte(content))
		return err
	}

	if err := write("[Content_Types].xml", contentTypesXML()); err != nil { return nil, err }
	if err := write("_rels/.rels", rootRelsXML()); err != nil { return nil, err }
	if err := write("xl/workbook.xml", workbookXML()); err != nil { return nil, err }
	if err := write("xl/_rels/workbook.xml.rels", workbookRelsXML()); err != nil { return nil, err }
	if err := write("xl/styles.xml", stylesXML()); err != nil { return nil, err }
	if err := write("xl/sharedStrings.xml", buildSharedStrings(s)); err != nil { return nil, err }
	if err := write("xl/worksheets/sheet1.xml", sheetContent); err != nil { return nil, err }
	if err := write("xl/worksheets/_rels/sheet1.xml.rels", sheetRelsContent); err != nil { return nil, err }

	zw.Close()
	return buf.Bytes(), nil
}

// ─── XML generators ───────────────────────────────────────────────────────

func contentTypesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
		`<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
		`</Types>`
}

func rootRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`
}

func workbookXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<sheets><sheet name="Danh sách video" sheetId="1" r:id="rId1"/></sheets>` +
		`</workbook>`
}

func workbookRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`</Relationships>`
}

func stylesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<fonts count="6">` +
		`<font><sz val="11"/><name val="Arial"/><color rgb="FF333355"/></font>` +
		`<font><sz val="13"/><b/><name val="Arial"/><color rgb="FFF5A623"/></font>` +
		`<font><sz val="11"/><b/><name val="Arial"/><color rgb="FFFFFFFF"/></font>` +
		`<font><sz val="10"/><name val="Arial"/><color rgb="FFF5A623"/><u val="single"/></font>` +
		`<font><sz val="11"/><b/><name val="Arial"/><color rgb="FFF5A623"/></font>` +
		`<font><sz val="10"/><name val="Arial"/><color rgb="FF555577"/></font>` +
		`</fonts>` +
		`<fills count="6">` +
		`<fill><patternFill patternType="none"/></fill>` +
		`<fill><patternFill patternType="gray125"/></fill>` +
		`<fill><patternFill patternType="solid"><fgColor rgb="FF1A1A2E"/></patternFill></fill>` +
		`<fill><patternFill patternType="solid"><fgColor rgb="FF22223A"/></patternFill></fill>` +
		`<fill><patternFill patternType="solid"><fgColor rgb="FFEEEEF8"/></patternFill></fill>` +
		`<fill><patternFill patternType="solid"><fgColor rgb="FFFAFAFA"/></patternFill></fill>` +
		`</fills>` +
		`<borders count="2">` +
		`<border><left/><right/><top/><bottom/></border>` +
		`<border>` +
		`<left style="thin"><color rgb="FFB0B0CC"/></left>` +
		`<right style="thin"><color rgb="FFB0B0CC"/></right>` +
		`<top style="thin"><color rgb="FFB0B0CC"/></top>` +
		`<bottom style="thin"><color rgb="FFB0B0CC"/></bottom>` +
		`</border>` +
		`</borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="8">` +
		// 0: data dark
		`<xf numFmtId="0" fontId="0" fillId="3" borderId="1" xfId="0"><alignment vertical="center"/></xf>` +
		// 1: title
		`<xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0"><alignment horizontal="center" vertical="center" wrapText="1"/></xf>` +
		// 2: header
		`<xf numFmtId="0" fontId="2" fillId="2" borderId="1" xfId="0"><alignment horizontal="center" vertical="center"/></xf>` +
		// 3: link
		`<xf numFmtId="0" fontId="3" fillId="3" borderId="1" xfId="0"><alignment vertical="center"/></xf>` +
		// 4: total
		`<xf numFmtId="0" fontId="4" fillId="2" borderId="1" xfId="0"><alignment horizontal="center" vertical="center"/></xf>` +
		// 5: center dark
		`<xf numFmtId="0" fontId="0" fillId="3" borderId="1" xfId="0"><alignment horizontal="center" vertical="center"/></xf>` +
		// 6: zebra light
		`<xf numFmtId="0" fontId="5" fillId="4" borderId="1" xfId="0"><alignment vertical="center"/></xf>` +
		// 7: zebra light center
		`<xf numFmtId="0" fontId="5" fillId="4" borderId="1" xfId="0"><alignment horizontal="center" vertical="center"/></xf>` +
		`</cellXfs>` +
		`</styleSheet>`
}

func buildSharedStrings(s *xlsxSheet) string {
	n := len(s.sharedStrs)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(fmt.Sprintf(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`, n, n))
	for _, v := range s.sharedStrs {
		sb.WriteString(`<si><t xml:space="preserve">`)
		sb.WriteString(xmlEsc(v))
		sb.WriteString(`</t></si>`)
	}
	sb.WriteString(`</sst>`)
	return sb.String()
}

func buildSheetXML(s *xlsxSheet) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)

	// 1. sheetViews (freeze panes) — must come before cols
	if s.freezeRow > 0 {
		sb.WriteString(fmt.Sprintf(
			`<sheetViews><sheetView workbookViewId="0"><pane ySplit="%d" topLeftCell="A%d" activePane="bottomLeft" state="frozen"/><selection pane="bottomLeft"/></sheetView></sheetViews>`,
			s.freezeRow, s.freezeRow+1))
	}

	// 2. cols
	if len(s.colWidths) > 0 {
		sb.WriteString("<cols>")
		for i, w := range s.colWidths {
			sb.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%.2f" customWidth="1"/>`, i+1, i+1, w))
		}
		sb.WriteString("</cols>")
	}

	// 3. sheetData
	hlIdx := 0
	sb.WriteString("<sheetData>")
	for rowIdx, row := range s.rows {
		rNum := rowIdx + 1
		h := row.height
		if h == 0 { h = 18 }
		sb.WriteString(fmt.Sprintf(`<row r="%d" ht="%.1f" customHeight="1">`, rNum, h))

		for colIdx, c := range row.cells {
			ref := fmt.Sprintf("%s%d", colLetter(colIdx+1), rNum)
			st  := c.styleID

			if c.isNum {
				sb.WriteString(fmt.Sprintf(`<c r="%s" s="%d"><v>%g</v></c>`, ref, st, c.numVal))
			} else if c.value == "" {
				sb.WriteString(fmt.Sprintf(`<c r="%s" s="%d"/>`, ref, st))
			} else {
				si := s.intern(c.value)
				sb.WriteString(fmt.Sprintf(`<c r="%s" t="s" s="%d"><v>%d</v></c>`, ref, st, si))
				if c.isLink && c.linkURL != "" {
					hlIdx++
					s.hyperlinks = append(s.hyperlinks, hlEntry{
						ref: ref,
						url: c.linkURL,
						rid: fmt.Sprintf("rHl%d", hlIdx),
					})
				}
			}
		}
		sb.WriteString("</row>")
	}
	sb.WriteString("</sheetData>")

	// 4. autoFilter
	if len(s.rows) > 3 {
		last := fmt.Sprintf("%s%d", colLetter(len(s.colWidths)), len(s.rows)-1)
		sb.WriteString(fmt.Sprintf(`<autoFilter ref="A3:%s"/>`, last))
	}

	// 5. hyperlinks
	if len(s.hyperlinks) > 0 {
		sb.WriteString("<hyperlinks>")
		for _, hl := range s.hyperlinks {
			sb.WriteString(fmt.Sprintf(`<hyperlink ref="%s" r:id="%s"/>`, hl.ref, hl.rid))
		}
		sb.WriteString("</hyperlinks>")
	}

	sb.WriteString("</worksheet>")
	return sb.String()
}

func buildSheetRels(s *xlsxSheet) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, hl := range s.hyperlinks {
		sb.WriteString(fmt.Sprintf(
			`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="%s" TargetMode="External"/>`,
			hl.rid, xmlEsc(hl.url)))
	}
	sb.WriteString("</Relationships>")
	return sb.String()
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func colLetter(n int) string {
	r := ""
	for n > 0 {
		n--
		r = string(rune('A'+n%26)) + r
		n /= 26
	}
	return r
}

func xmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

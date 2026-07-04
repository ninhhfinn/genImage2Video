package main

// Dump dữ liệu THẬT đã parse từ API (testdata/listings.json) qua CHÍNH parseListings
// để biết các chuỗi tên/địa chỉ/giá/tiện nghi mà 8 template video sẽ nhận.
// Run: GENDUMP=1 go test -count=1 -run TestDumpRealListings .

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDumpRealListings(t *testing.T) {
	if os.Getenv("GENDUMP") == "" {
		t.Skip("set GENDUMP=1")
	}
	data, err := os.ReadFile("testdata/listings.json")
	if err != nil {
		t.Fatal(err)
	}
	infos := parseListings(data)
	t.Logf("parsed %d listings", len(infos))
	for i, in := range infos {
		nick := in.Nickname
		if nick == "" {
			nick = "(nil)"
		}
		fmt.Printf("\n#%02d  nickname=%q  name=%q\n", i, nick, in.Name)
		fmt.Printf("     address=%q (len=%d)\n", in.Address, len([]rune(in.Address)))
		fmt.Printf("     photos=%d\n", len(in.PhotoURLs))
		for _, p := range in.PriceLines {
			fmt.Printf("       price: %q (len=%d)\n", p, len([]rune(p)))
		}
		am := in.Amenities
		if len(am) > 8 {
			am = am[:8]
		}
		fmt.Printf("     amenities(%d): %s\n", len(in.Amenities), strings.Join(am, " | "))
	}
}

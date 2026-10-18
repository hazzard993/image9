//go:build windows

package main

import (
	"errors"
	"testing"
)

func TestColourSpaceNeutralAndAlpha(t *testing.T) {
	for _, space := range []sourceColourSpace{spaceP3, spaceAdobeRGB, spaceRec2020} {
		im := &decodedImage{width: 3, height: 1, pixels: []byte{0, 0, 0, 255, 255, 255, 255, 255, 64, 64, 64, 128}}
		if err := convertSourceColourSpace(im, space, func() bool { return false }); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			if im.pixels[i] != 0 || im.pixels[4+i] < 254 {
				t.Fatalf("space %d changed black/white: %v", space, im.pixels)
			}
		}
		if im.pixels[11] != 128 {
			t.Fatal("conversion changed alpha")
		}
		for i := 8; i < 11; i++ {
			if im.pixels[i] > 128 {
				t.Fatal("invalid premultiplied pixel")
			}
		}
		if delta := int(im.pixels[8]) - int(im.pixels[10]); delta < -1 || delta > 1 {
			t.Fatal("neutral acquired colour cast")
		}
	}
}

func TestP3ConversionAndSRGBPassthrough(t *testing.T) {
	original := []byte{64, 64, 128, 255, 128, 128, 128, 255}
	im := &decodedImage{width: 2, height: 1, pixels: append([]byte(nil), original...)}
	if err := convertSourceColourSpace(im, spaceSRGB, func() bool { return false }); err != nil || string(im.pixels) != string(original) {
		t.Fatal("sRGB should be unchanged")
	}
	if err := convertSourceColourSpace(im, spaceP3, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	if im.pixels[2] <= 128 || im.pixels[1] >= 64 {
		t.Fatalf("P3 colour was not converted to wider sRGB channel separation: %v", im.pixels)
	}
	for _, p := range im.pixels[4:7] {
		if p != 128 {
			t.Fatal("Display P3 grey should remain unchanged")
		}
	}
	if err := convertSourceColourSpace(im, spaceP3, func() bool { return true }); !errors.Is(err, errLoadSuperseded) {
		t.Fatal("conversion should honour cancellation")
	}
}

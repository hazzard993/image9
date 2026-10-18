//go:build windows

package main

import "testing"

func TestCopyColourFormats(t *testing.T) {
	for _, tc := range []struct {
		input  string
		format colourFormat
		want   string
	}{
		{"#123456", formatHex, "#123456"},
		{"#123456", formatRGB, "rgb(18, 52, 86)"},
		{"#FF000080", formatRGB, "rgba(255, 0, 0, 0.502)"},
		{"#FF0000", formatHSL, "hsl(0, 100.0%, 50.0%)"},
		{"#00FF00", formatHSL, "hsl(120, 100.0%, 50.0%)"},
		{"#0000FF", formatHSL, "hsl(240, 100.0%, 50.0%)"},
		{"#FFFFFF", formatHSL, "hsl(0, 0.0%, 100.0%)"},
		{"#00000000", formatHSL, "hsla(0, 0.0%, 0.0%, 0)"},
		{"#FF00FF80", formatHSL, "hsla(300, 100.0%, 50.0%, 0.502)"},
	} {
		if got := formatPickedColour(tc.input, tc.format); got != tc.want {
			t.Errorf("%s format %d: got %s want %s", tc.input, tc.format, got, tc.want)
		}
	}
}

func TestCopyFormatPreference(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if got := loadCopyFormat(); got != formatHex {
		t.Fatal("default format is not hex")
	}
	v := &viewer{}
	for _, format := range []colourFormat{formatRGB, formatHSL, formatHex} {
		v.setCopyFormat(format)
		if got := loadCopyFormat(); got != format {
			t.Fatalf("saved %d loaded %d", format, got)
		}
	}
}

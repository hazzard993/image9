//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type colourFormat int

const (
	formatHex colourFormat = iota
	formatRGB
	formatHSL
)

func formatPickedColour(hex string, format colourFormat) string {
	if format == formatHex || len(hex) != 7 && len(hex) != 9 || hex[0] != '#' {
		return hex
	}
	value, err := strconv.ParseUint(hex[1:7], 16, 32)
	if err != nil {
		return hex
	}
	r, g, b := int(value>>16), int(value>>8&255), int(value&255)
	alpha := ""
	if len(hex) == 9 {
		a, err := strconv.ParseUint(hex[7:], 16, 8)
		if err != nil {
			return hex
		}
		alpha = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", float64(a)/255), "0"), ".")
	}
	if format == formatRGB {
		if alpha != "" {
			return fmt.Sprintf("rgba(%d, %d, %d, %s)", r, g, b, alpha)
		}
		return fmt.Sprintf("rgb(%d, %d, %d)", r, g, b)
	}
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	maximum, minimum := math.Max(rf, math.Max(gf, bf)), math.Min(rf, math.Min(gf, bf))
	delta := maximum - minimum
	h, s, l := 0.0, 0.0, (maximum+minimum)/2
	if delta != 0 {
		s = delta / (1 - math.Abs(2*l-1))
		switch maximum {
		case rf:
			h = math.Mod((gf-bf)/delta, 6)
		case gf:
			h = (bf-rf)/delta + 2
		default:
			h = (rf-gf)/delta + 4
		}
		h = math.Mod(h*60+360, 360)
	}
	hue := int(math.Round(h)) % 360
	if alpha != "" {
		return fmt.Sprintf("hsla(%d, %.1f%%, %.1f%%, %s)", hue, s*100, l*100, alpha)
	}
	return fmt.Sprintf("hsl(%d, %.1f%%, %.1f%%)", hue, s*100, l*100)
}

func copyFormatPath() string {
	base := themeConfigPath()
	if base == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(base), "colour-format.txt")
}
func loadCopyFormat() colourFormat {
	data, _ := readAppPreference("colour-format.txt")
	switch strings.TrimSpace(string(data)) {
	case "rgb":
		return formatRGB
	case "hsl":
		return formatHSL
	default:
		return formatHex
	}
}
func (v *viewer) setCopyFormat(format colourFormat) {
	v.copyFormat = format
	for i, action := range v.copyFormatActions {
		if action != nil {
			action.SetChecked(i == int(format))
		}
	}
	path := copyFormatPath()
	if path != "" && os.MkdirAll(filepath.Dir(path), 0700) == nil {
		_ = os.WriteFile(path, []byte([]string{"hex", "rgb", "hsl"}[format]+"\n"), 0600)
	}
}

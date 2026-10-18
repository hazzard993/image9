//go:build windows

package main

import "testing"

func TestModeText(t *testing.T) {
	input := ""
	for _, test := range []struct {
		state viewer
		want  string
	}{
		{viewer{}, ""},
		{viewer{highlightBounds: true}, "Inspect Mode"},
		{viewer{ctrlViewPinned: true}, "Inspect Mode"},
		{viewer{gapInput: &input}, "Input Mode"},
		{viewer{placingWrap: true}, "Input Mode"},
		{viewer{highlightBounds: true, gapInput: &input}, "Input Mode"},
	} {
		if got := test.state.modeText(); got != test.want {
			t.Fatalf("mode %q, want %q", got, test.want)
		}
	}
}

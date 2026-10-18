//go:build windows

package main

import (
	"github.com/lxn/walk"
	"testing"
)

func TestPixelExpressions(t *testing.T) {
	for _, test := range []struct {
		input         string
		current, want int
	}{
		{"64*3+8*2", 0, 208}, {"(64+8)*3-8", 0, 208}, {"*2", 32, 64}, {"/2", 32, 16},
		{"-8", 32, 24}, {"+8", 32, 40}, {"100/3", 0, 33}, {"2.5*4", 0, 10}, {"10%3", 0, 1},
		{"08+002", 0, 10}, {"", 32, 0}, {"32-32", 0, 0},
	} {
		if got, err := pixelExpression(test.input, test.current); err != nil || got != test.want {
			t.Fatalf("%q: got %d, %v; want %d", test.input, got, err, test.want)
		}
	}
	for _, input := range []string{"1/0", "5%0", "1-2", "1000001", "(8+)", "2**3", "hello", "2+", ".5.2"} {
		if _, err := pixelExpression(input, 32); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestPixelOperatorKeys(t *testing.T) {
	for _, test := range []struct {
		key  walk.Key
		mods walk.Modifiers
		want byte
	}{
		{walk.Key8, walk.ModShift, '*'}, {walk.Key9, walk.ModShift, '('}, {walk.Key0, walk.ModShift, ')'},
		{walk.KeyOEMPlus, walk.ModShift, '+'}, {walk.KeyDivide, 0, '/'}, {walk.KeySubtract, walk.ModControl, '-'},
		{walk.Key4, walk.ModShift, '$'},
	} {
		if got, ok := pixelInputCharacter(test.key, test.mods); !ok || got != test.want {
			t.Fatalf("operator key %v: %q %v", test.key, got, ok)
		}
	}
}

func TestImageWidthReferences(t *testing.T) {
	for _, test := range []struct {
		input string
		want  int
	}{
		{"$1", 64}, {"$2", 128}, {"$1+$2+8", 200}, {"$2/$1*16", 32}, {"*$1", 2048},
	} {
		if got, err := pixelExpression(test.input, 32, 64, 128); err != nil || got != test.want {
			t.Fatalf("%q: %d, %v; want %d", test.input, got, err, test.want)
		}
	}
	for _, input := range []string{"$0", "$3", "$", "$1$2", "$1.5", "$999999999999999999999"} {
		if _, err := pixelExpression(input, 0, 64, 128); err == nil {
			t.Fatalf("accepted invalid reference %q", input)
		}
	}
	if _, err := pixelExpression("$2", 0, 64, 0); err == nil {
		t.Fatal("accepted width of an unloaded image")
	}
}

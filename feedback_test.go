//go:build windows

package main

import (
	"testing"
	"time"
)

func TestFeedbackLifetime(t *testing.T) {
	for _, elapsed := range []time.Duration{0, 2 * time.Second, 2499 * time.Millisecond} {
		if feedbackOpacity(elapsed) != 1 {
			t.Fatalf("message faded or disappeared at %v", elapsed)
		}
	}
	for _, elapsed := range []time.Duration{2500 * time.Millisecond, 10 * time.Second} {
		if feedbackOpacity(elapsed) != 0 {
			t.Fatalf("message remained visible at %v", elapsed)
		}
	}
}

//go:build windows

package main

import (
	"time"
)

const feedbackTimerID = 0x51495632
const feedbackDuration = 2500 * time.Millisecond

var setFeedbackTimer = userDLL.NewProc("SetTimer")
var killFeedbackTimer = userDLL.NewProc("KillTimer")

func feedbackOpacity(elapsed time.Duration) float32 {
	if elapsed >= feedbackDuration {
		return 0
	}
	return 1
}

func (v *viewer) notifyAction(text string) {
	v.feedbackText = text
	v.feedbackTime = time.Now()
	if v.window != nil {
		setFeedbackTimer.Call(uintptr(v.window.Handle()), feedbackTimerID, uintptr(feedbackDuration/time.Millisecond), 0)
	}
	v.canvas.Invalidate()
}

func (v *viewer) tickFeedback() {
	if v.gapInput != nil || v.placingWrap {
		return
	}
	if feedbackOpacity(time.Since(v.feedbackTime)) == 0 {
		v.feedbackText = ""
		killFeedbackTimer.Call(uintptr(v.window.Handle()), feedbackTimerID)
	}
	v.canvas.Invalidate()
}

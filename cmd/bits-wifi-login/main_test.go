package main

import (
	"testing"
	"time"
)

func probes(script ...bool) func() bool {
	i := 0

	return func() bool {
		defer func() { i++ }()

		if i < len(script) {
			return script[i]
		}

		return true
	}
}

// The 2026-09-04 04:40 failure, see confirmDrop.
func TestWatchLoopKeepsWatchingAfterAFalseAlarm(t *testing.T) {
	calls := 0
	login := func() bool {
		calls++

		return calls > 1
	}

	if !watchLoop(time.Now().Add(time.Minute), time.Millisecond,
		probes(true, false, false, true, true, false, false), login) {
		t.Fatal("watchLoop gave up on the false alarm instead of catching the real drop")
	}

	if calls != 2 {
		t.Fatalf("login attempts = %d, want 2", calls)
	}
}

func TestWatchLoopIgnoresASingleMissedProbe(t *testing.T) {
	login := func() bool {
		t.Fatal("logged in after one failed probe")

		return true
	}

	if watchLoop(time.Now().Add(30*time.Millisecond), time.Millisecond,
		probes(true, false, true, false, true), login) {
		t.Fatal("watchLoop reported a drop it never confirmed")
	}
}

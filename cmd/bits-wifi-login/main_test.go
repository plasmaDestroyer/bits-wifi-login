package main

import (
	"testing"
	"time"
)

// probes returns a probe func that answers from a script, then stays online.
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

// The 2026-09-04 04:40 failure: WARP's DNS proxy timed out resolving the probe
// host at 04:40:30, the network answered 204 three seconds later, and the
// watcher exited on that healed blip — nine seconds before the session really
// expired at 04:40:39. Nothing was left camping on it.
func TestWatchLoopKeepsWatchingAfterAFalseAlarm(t *testing.T) {
	calls := 0
	login := func() bool {
		calls++

		return calls > 1 // the first is the blip that healed by itself
	}

	if !watchLoop(time.Now().Add(time.Minute), time.Millisecond,
		probes(true, false, false, true, true, false, false), login) {
		t.Fatal("watchLoop gave up on the false alarm instead of catching the real drop")
	}

	if calls != 2 {
		t.Fatalf("login attempts = %d, want 2", calls)
	}
}

// A single miss is a hiccup, not an expiry: acting on one is what let a DNS
// timeout stand in for a session drop.
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

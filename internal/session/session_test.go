package session

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestShouldWatch(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	// Deadline 13:00: window opens 12:50, grace ends 13:05.
	s := Session{LoginAt: now.Add(-3 * time.Hour), Timeout: 4 * time.Hour}

	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"hours out", now, false},
		{"a minute before the window", now.Add(49 * time.Minute), false},
		{"window opens", now.Add(50 * time.Minute), true},
		{"inside the window", now.Add(55 * time.Minute), true},
		{"at the deadline", now.Add(time.Hour), true},
		{"inside the grace", now.Add(time.Hour + 4*time.Minute), true},
		{"grace expired", now.Add(time.Hour + 5*time.Minute), false},
		{"long past", now.Add(9 * time.Hour), false},
	}

	for _, c := range cases {
		if got := ShouldWatch(c.at, s); got != c.want {
			t.Errorf("%s: ShouldWatch() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestShouldWatchZeroSession(t *testing.T) {
	for _, s := range []Session{
		{},
		{LoginAt: time.Now()},              // no timeout
		{Timeout: 4 * time.Hour},           // no login time
		{LoginAt: time.Now(), Timeout: -1}, // nonsense timeout
	} {
		if ShouldWatch(time.Now(), s) {
			t.Errorf("ShouldWatch(%+v) = true, want false", s)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := Session{LoginAt: time.Now().Truncate(time.Second).UTC(), Timeout: 14400 * time.Second}

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}

	if got := Load(path); !got.LoginAt.Equal(want.LoginAt) || got.Timeout != want.Timeout {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}

	if runtime.GOOS == "windows" {
		return // no unix mode bits. See the comment on creds.Load
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestLoadSurvivesGarbage(t *testing.T) {
	dir := t.TempDir()

	for name, content := range map[string]string{
		"missing.json":   "",
		"truncated.json": `{"login_at":`,
		"wrongtype.json": `{"login_at": 12345, "timeout": "nope"}`,
		"empty.json":     "",
		"notjson.json":   "hello",
	} {
		path := filepath.Join(dir, name)
		if name != "missing.json" {
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}

		if got := Load(path); !got.LoginAt.IsZero() || got.Timeout != 0 {
			t.Errorf("%s: Load() = %+v, want zero session", name, got)
		}
	}
}

func TestLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.lock")

	release, ok := Lock(path)
	if !ok {
		t.Fatal("first Lock() = false, want true")
	}

	if _, ok := Lock(path); ok {
		t.Error("second Lock() = true, want false: two watchers would race to log in")
	}

	release()

	if _, ok := Lock(path); !ok {
		t.Error("Lock() after release = false, want true")
	}
}

func TestLockIgnoresStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.lock")

	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-(WatchWindow + WatchGrace + time.Minute))
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if _, ok := Lock(path); !ok {
		t.Error("Lock() on a stale lock = false, want true")
	}
}

func TestObserve(t *testing.T) {
	base := time.Date(2026, 8, 27, 15, 24, 13, 0, time.UTC)
	day := 24*time.Hour + 3*time.Minute

	cases := []struct {
		name string
		prev Session
		at   time.Time
		want time.Duration
	}{
		{
			name: "first ever login measures nothing",
			prev: Session{},
			at:   base,
			want: 0,
		},
		{
			name: "one drop gives the first estimate",
			prev: Session{LoginAt: base},
			at:   base.Add(day),
			want: day,
		},
		{
			name: "a longer observation never loosens the estimate",
			prev: Session{LoginAt: base, Timeout: day},
			at:   base.Add(3 * day),
			want: day,
		},
		{
			name: "a shorter plausible observation tightens it",
			prev: Session{LoginAt: base, Timeout: day},
			at:   base.Add(5 * time.Hour),
			want: 5 * time.Hour,
		},
		{
			name: "an implausibly short observation is rejected",
			prev: Session{LoginAt: base, Timeout: day},
			at:   base.Add(4 * time.Minute),
			want: day,
		},
		{
			name: "and cannot seed the estimate either",
			prev: Session{LoginAt: base},
			at:   base.Add(time.Minute),
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Observe(c.prev, c.at)

			if got.Timeout != c.want {
				t.Errorf("Observe() timeout = %v, want %v", got.Timeout, c.want)
			}
			if !got.LoginAt.Equal(c.at) {
				t.Errorf("Observe() LoginAt = %v, want %v", got.LoginAt, c.at)
			}
		})
	}
}

func TestObserveWithoutAnEstimateDisablesTheWatch(t *testing.T) {
	at := time.Date(2026, 8, 28, 15, 30, 11, 0, time.UTC)

	if s := Observe(Session{}, at); ShouldWatch(at, s) {
		t.Errorf("ShouldWatch(%v) = true for an unmeasured session, want false", s)
	}
}

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
	// Deadline is now + 1h, so the window opens at 12:50 and the grace ends at 13:05.
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

// An unknown deadline must never trigger a watch — a zero session is what a
// missing or corrupt state file loads as.
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
		return // no unix mode bits; see the comment on creds.Load
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

// Load is the one thing in this package that absolutely cannot fail: a bad state
// file must degrade to "deadline unknown", never block the login path.
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
		t.Error("second Lock() = true, want false — two watchers would race to log in")
	}

	release()

	if _, ok := Lock(path); !ok {
		t.Error("Lock() after release = false, want true")
	}
}

// A killed watcher leaves its lock behind. If that were honoured forever, one
// SIGKILL would disable the feature until somebody noticed a stray file.
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

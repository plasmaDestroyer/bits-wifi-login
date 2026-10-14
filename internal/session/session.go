// Package session remembers when the portal let us in, so the next expiry can
// be seen coming.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// Must be >= the coarsest trigger interval, or the watcher never arms.
	WatchWindow = 10 * time.Minute
	WatchGrace  = 5 * time.Minute
	// Only paid inside the watch window.
	PollInterval = 500 * time.Millisecond
	// Two logins this close are a failed attempt, not a session lifetime.
	MinLifetime = 30 * time.Minute
)

// No token on purpose: a stale state file can never log out a healthy session.
type Session struct {
	LoginAt time.Time     `json:"login_at"`
	Timeout time.Duration `json:"timeout"`
}

func (s Session) Deadline() time.Time {
	return s.LoginAt.Add(s.Timeout)
}

// Observe folds a fresh login into the measured session lifetime (12h, the
// portal's advertised 14400s is wrong). Keeps the SMALLEST plausible gap:
// suspend, a missed drop or a coarse trigger can only stretch a gap.
func Observe(prev Session, loginAt time.Time) Session {
	next := Session{LoginAt: loginAt, Timeout: prev.Timeout}

	if prev.LoginAt.IsZero() {
		return next
	}

	observed := loginAt.Sub(prev.LoginAt)
	if observed >= MinLifetime && (next.Timeout <= 0 || observed < next.Timeout) {
		next.Timeout = observed
	}

	return next
}

// ShouldWatch reports whether now is close enough to the expiry to camp on it.
func ShouldWatch(now time.Time, s Session) bool {
	if s.LoginAt.IsZero() || s.Timeout <= 0 {
		return false
	}

	deadline := s.Deadline()

	return !now.Before(deadline.Add(-WatchWindow)) && now.Before(deadline.Add(WatchGrace))
}

// Beside the binary, like creds.conf.
func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "session.json"
	}

	return filepath.Join(filepath.Dir(exe), "session.json")
}

// Load never fails: an unknown deadline must never block a login.
func Load(path string) Session {
	content, err := os.ReadFile(path)
	if err != nil {
		return Session{}
	}

	var s Session
	if err := json.Unmarshal(content, &s); err != nil {
		return Session{}
	}

	return s
}

func Save(path string, s Session) error {
	content, err := json.Marshal(s)
	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0600)
}

// Lock stops the timer and the NM dispatcher camping at once (systemd only
// serialises its own unit). A lock older than a whole watch belongs to a
// killed process and gets taken over.
func Lock(path string) (release func(), ok bool) {
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < WatchWindow+WatchGrace {
		return nil, false
	}

	os.Remove(path)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, false
	}
	f.Close()

	return func() { os.Remove(path) }, true
}

func LockPath() string {
	return filepath.Join(filepath.Dir(DefaultPath()), "session.lock")
}

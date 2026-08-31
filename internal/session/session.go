// Package session remembers when the portal let us in, so the next expiry can be
// anticipated instead of waited for.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// Must be >= the coarsest trigger interval, or no trigger lands inside it and
	// the watcher never runs.
	WatchWindow  = 10 * time.Minute
	WatchGrace   = 5 * time.Minute
	PollInterval = 2 * time.Second
	// Rejects an implausibly short observation: two logins close together are a
	// failed attempt, and one such sample would pin the estimate low for good.
	MinLifetime = 30 * time.Minute
)

// Deliberately no token: the watcher never talks to the portal about an existing
// session, so a stale state file cannot disconnect a healthy one.
type Session struct {
	LoginAt time.Time     `json:"login_at"`
	Timeout time.Duration `json:"timeout"`
}

func (s Session) Deadline() time.Time {
	return s.LoginAt.Add(s.Timeout)
}

// Observe folds a fresh login into what we know about how long a session lasts.
// Measured, never believed from the portal, whose advertised 14400s is not the
// lifetime (it is 12h — see CLAUDE.md).
//
// The estimate is the SMALLEST plausible gap ever seen, because every error
// source runs one way: suspend, an undetected drop and a coarse trigger interval
// can only stretch an observed gap, so the minimum converges from above.
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

// ShouldWatch reports whether now is close enough to the predicted expiry to be
// worth camping on. Pure, so the window arithmetic is testable without clocks or
// sockets.
func ShouldWatch(now time.Time, s Session) bool {
	if s.LoginAt.IsZero() || s.Timeout <= 0 {
		return false
	}

	deadline := s.Deadline()

	return !now.Before(deadline.Add(-WatchWindow)) && now.Before(deadline.Add(WatchGrace))
}

// Beside the binary, like creds.conf: a trigger has no useful working directory.
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

// Lock stops two watchers camping at once: the timer and the NM dispatcher
// invoke the binary independently, so systemd's one-instance-per-unit does not
// cover it. A lock older than a whole watch can only be a killed process and is
// taken over, or one SIGKILL would wedge the feature permanently.
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

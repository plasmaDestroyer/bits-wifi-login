// Package session remembers when the portal let us in, so the next expiry can be
// anticipated instead of waited for. The portal answers nothing about a live
// session — every path empty-replies while authenticated — so the deadline has to
// be kept here or not at all.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// WatchWindow must be at least as long as the coarsest trigger interval, or no
	// trigger ever lands inside it and the watcher never runs. Linux ticks every
	// 10 min, macOS and Windows every 5.
	WatchWindow = 10 * time.Minute
	// WatchGrace keeps the watcher alive past its own prediction, since a deadline
	// that runs a little late is the normal case and giving up exactly on time
	// would hand every one of those back to the slow path.
	WatchGrace = 5 * time.Minute
	// PollInterval is how often the watcher asks whether the session has dropped.
	PollInterval = 2 * time.Second
	// MinLifetime rejects an implausibly short observation. Two logins close
	// together mean a failed attempt or a flapping link, not a session that
	// genuinely lasted that long, and one bad sample would otherwise pin the
	// estimate low for good.
	MinLifetime = 30 * time.Minute
)

// Session is the persisted form of portal.Session. Deliberately no token: the
// watcher never talks to the portal about an existing session, which is what
// keeps a stale state file from being able to disconnect a healthy one.
type Session struct {
	LoginAt time.Time     `json:"login_at"`
	Timeout time.Duration `json:"timeout"`
}

func (s Session) Deadline() time.Time {
	return s.LoginAt.Add(s.Timeout)
}

// Observe folds a fresh login into what we know about how long a session lasts.
//
// The portal advertises 14400s on its keepalive page and that number is simply
// not the session lifetime: measured 2026-08-28, a session logged in at 15:30
// was still alive at 20:29 with the watcher having polled straight through the
// predicted 19:30 deadline and found nothing. The one drop we have both ends of
// ran 24h03m. So the lifetime is measured here rather than believed from the
// portal, which also means a change of campus policy needs no code change.
//
// The estimate is the SMALLEST plausible gap ever seen, because every source of
// error runs one way: a suspended laptop, a closed lid, an undetected drop and a
// coarse trigger interval can only make an observed gap longer than the true
// lifetime, never shorter. Taking the minimum converges on the truth from above.
// Being wrong low is the safe direction anyway — the watcher arms early, finds
// nothing, and gives up.
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

// DefaultPath puts the state beside the binary, for the same reason creds.conf
// lives there: a systemd/launchd/Task Scheduler trigger has no useful working
// directory.
func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "session.json"
	}

	return filepath.Join(filepath.Dir(exe), "session.json")
}

// Load never fails. A missing, unreadable or corrupt state file means only that
// the deadline is unknown, and an unknown deadline must never be able to block a
// login — the whole feature is an optimisation over a path that already works.
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

// Lock stops two watchers camping at once. The systemd timer and the NM
// dispatcher invoke the binary independently — the dispatcher bypasses systemd
// entirely — so relying on systemd's one-instance-per-unit behaviour would not
// cover it. Returns nil and a false ok when someone else already holds it.
//
// A lock older than a whole watch can only be a killed process, so it is taken
// over rather than honoured; otherwise one SIGKILL would wedge the feature until
// someone noticed a file.
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

// Command bits-wifi-login logs into the BITS Pilani Fortinet captive portal.
// With no arguments it is what the background triggers run, and exits 0 when
// there is nothing to do.
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/installer"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/portal"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/runlog"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/session"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/wifi"
)

const (
	// The portal EOFs mid-flow often enough that two quick tries burned out
	// before it recovered. Backed off instead: 3s, 6s, 12s.
	attempts = 4
	// The session takes a moment to go live. Polled, not slept.
	settleTimeout = 3 * time.Second
	settleStep    = 250 * time.Millisecond
	retryDelay    = 3 * time.Second
)

// version is stamped by the release build (-ldflags -X main.version=<tag>).
var version = "dev"

func main() {
	// Dated: these logs get read days later.
	log.SetFlags(log.Ldate | log.Ltime)

	var command string
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "":
		login()
	case "install":
		fatal(installer.Install())
	case "uninstall":
		fatal(installer.Uninstall())
	case "status":
		status()
	case "version", "-v", "--version":
		fmt.Println(version)
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", command)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `bits-wifi-login: auto-login for the BITS Pilani captive portal

  bits-wifi-login              log in now (what the background triggers run)
  bits-wifi-login install      set up credentials and background triggers
  bits-wifi-login uninstall    remove the background triggers
  bits-wifi-login status       where everything lives and whether it is working
  bits-wifi-login version      which release this binary is
`)
}

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func status() {
	fmt.Println("bits-wifi-login " + version)

	fmt.Println("\n  Files")
	for _, path := range installer.Files() {
		fmt.Printf("    %-12s%s\n", exists(path), path)
	}

	fmt.Println("\n  Triggers")
	for _, t := range installer.Triggers() {
		state := "missing"
		if t.Registered {
			state = "registered"
		}
		fmt.Printf("    %-12s%s\n", state, t.Name)
	}

	fmt.Println("\n  Now")
	fmt.Printf("    %-12s%s\n", "network", network())
	fmt.Printf("    %-12s%s\n", "portal", authentication())

	if last := runlog.LastLine(); last != "" {
		fmt.Printf("    %-12s%s\n", "last run", last)
	}

	fmt.Println()
}

func exists(path string) string {
	if _, err := os.Stat(path); err != nil {
		return "missing"
	}

	return "present"
}

func network() string {
	ssid, err := wifi.SSID()
	switch {
	case err != nil:
		return fmt.Sprintf("could not be read (%v)", err)
	case ssid == "":
		return "not associated with any Wi-Fi network"
	case wifi.IsBITS(ssid):
		return fmt.Sprintf("%q (a BITS network)", ssid)
	default:
		return fmt.Sprintf("%q (not a BITS network)", ssid)
	}
}

func authentication() string {
	if portal.New().IsLoggedIn() {
		return "authenticated"
	}

	return "not logged in"
}

func login() {
	sink := runlog.Open()
	if sink != nil {
		defer sink.Close()

		log.SetOutput(io.MultiWriter(os.Stderr, sink))
		log.SetFlags(log.Ldate | log.Ltime)
	}

	// Progress chatter is for a human watching. A log file gets exactly one
	// outcome line per run, since that is the only proof the run happened.
	// stderr because that is where log writes.
	interactive := term.IsTerminal(int(os.Stderr.Fd()))
	verbose := interactive || sink != nil

	ssid, err := wifi.SSID()
	if err != nil {
		log.Fatalf("could not determine the current network: %v", err)
	}
	if !wifi.IsBITS(ssid) {
		if verbose {
			log.Printf("current WiFi is %q, not a BITS network. Skipping.", ssid)
		}
		return
	}

	p := portal.New()

	if interactive {
		log.Print("checking connectivity...")
	}
	if p.IsLoggedIn() {
		if verbose {
			log.Print("already authenticated, nothing to do.")
		}

		watch(p)
		return
	}

	authenticate(p, ssid)
}

// watch camps on a session about to expire. Best-effort: anything odd just
// returns and the reactive triggers pick it up.
func watch(p *portal.Portal) {
	s := session.Load(session.DefaultPath())

	if !session.ShouldWatch(time.Now(), s) && os.Getenv("BITS_WIFI_WATCH_NOW") != "1" {
		return
	}

	release, ok := session.Lock(session.LockPath())
	if !ok {
		return
	}
	defer release()

	deadline := s.Deadline()
	giveUp := deadline.Add(session.WatchGrace)
	if os.Getenv("BITS_WIFI_WATCH_NOW") == "1" {
		giveUp = time.Now().Add(session.WatchGrace)
	}

	log.Printf("expiry due %s, watching until %s",
		deadline.Format(time.TimeOnly), giveUp.Format(time.TimeOnly))

	if watchLoop(giveUp, session.PollInterval, p.IsLoggedIn, func() bool { return authenticate(p, "") }) {
		return
	}

	log.Print("expiry did not arrive within the grace period.")
}

// confirmDrop consecutive failed probes count as an expiry. One is not enough:
// on 2026-09-04 a single WARP DNS timeout retired the watcher nine seconds
// before the real expiry.
const confirmDrop = 2

// watchLoop polls until the session drops and reports whether it caught it.
//
// login returning false means the outage healed on its own. Keep camping: the
// expiry this watcher was armed for has not happened yet.
func watchLoop(giveUp time.Time, poll time.Duration, probe, login func() bool) bool {
	misses := 0

	for time.Now().Before(giveUp) {
		time.Sleep(poll)

		if probe() {
			misses = 0

			continue
		}

		if misses++; misses < confirmDrop {
			continue
		}

		if login() {
			return true
		}

		misses = 0
	}

	return false
}

// authenticate logs in. False means the network turned out healthy after all,
// which the watcher treats as a false alarm, not success.
func authenticate(p *portal.Portal, ssid string) bool {
	c, err := creds.Load(creds.DefaultPath())
	if err != nil {
		log.Fatal(err)
	}

	if ssid != "" {
		log.Printf("not logged in. Authenticating to %s...", ssid)
	}

	// Logged here, not in watch, so the reactive path gets measured too.
	if prev := session.Load(session.DefaultPath()); !prev.LoginAt.IsZero() {
		off, when := time.Since(prev.Deadline()).Round(time.Second), "after"
		if off < 0 {
			off, when = -off, "before"
		}

		log.Printf("expiry noticed %s %s the predicted deadline of %s",
			off, when, prev.Deadline().Format(time.TimeOnly))
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		log.Printf("attempt %d/%d...", attempt, attempts)

		s, err := p.Login(c)
		if errors.Is(err, portal.ErrAlreadyOnline) {
			log.Print("connectivity came back on its own, nothing to do.")

			return false
		}
		if err != nil {
			log.Print(err)
		} else if settled(p) {
			saved := session.Observe(session.Load(session.DefaultPath()), s.LoginAt)

			if saved.Timeout > 0 {
				log.Printf("login successful. Sessions last about %s, so the next expiry is due %s",
					saved.Timeout.Round(time.Minute), saved.Deadline().Format(time.DateTime))
			} else {
				log.Print("login successful. Session lifetime not measured yet.")
			}

			if err := session.Save(session.DefaultPath(), saved); err != nil {
				log.Printf("could not record the session deadline: %v", err)
			}

			return true
		} else {
			log.Print("portal accepted the login but there is still no connectivity.")
		}

		if attempt < attempts {
			time.Sleep(retryDelay << (attempt - 1))
		}
	}

	log.Fatal("all attempts failed.")

	return false
}

func settled(p *portal.Portal) bool {
	for waited := time.Duration(0); waited < settleTimeout; waited += settleStep {
		if p.IsLoggedIn() {
			return true
		}

		time.Sleep(settleStep)
	}

	return p.IsLoggedIn()
}

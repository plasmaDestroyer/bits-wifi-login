// Command bits-wifi-login authenticates against the BITS Pilani Fortinet captive
// portal. With no arguments it is meant to be fired by a background trigger
// (systemd timer, launchd agent, scheduled task) and exits 0 whenever there is
// nothing to do. `install` and `uninstall` manage those triggers, and `status`
// reports on them for the times when a silent background tool is
// indistinguishable from a broken one.
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
	// The portal drops connections mid-flow ("EOF" on the fgtauth GET or the
	// credential POST) often enough that two tries 3s apart burned out in six
	// seconds and left the network dead until the next 10-minute tick. Backed
	// off instead: 3s, 6s, 12s, ~21s of trying.
	attempts = 4
	// The portal needs a moment before the session is live. Polled, not slept:
	// this delay is paid on every login.
	settleTimeout = 3 * time.Second
	settleStep    = 250 * time.Millisecond
	retryDelay    = 3 * time.Second
)

func main() {
	// Date included: the dispatcher log is an append-only file read days later,
	// and bare clock times there are ambiguous exactly when it matters.
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
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", command)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `bits-wifi-login — auto-login for the BITS Pilani captive portal

  bits-wifi-login              log in now (what the background triggers run)
  bits-wifi-login install      set up credentials and background triggers
  bits-wifi-login uninstall    remove the background triggers
  bits-wifi-login status       where everything lives and whether it is working
`)
}

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// status answers the questions a background tool cannot answer by running:
// where it put itself, whether the triggers are still registered, and whether
// the last one that fired did anything. Without it the only evidence the tool
// exists is the absence of a captive portal.
func status() {
	fmt.Println("bits-wifi-login")

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
		return fmt.Sprintf("%q — a BITS network", ssid)
	default:
		return fmt.Sprintf("%q — not a BITS network, so a run would do nothing", ssid)
	}
}

func authentication() string {
	if portal.New().IsLoggedIn() {
		return "authenticated"
	}

	return "no connectivity — a run would try to log in"
}

func login() {
	// Where the output goes decides how much of it there is, so settle that
	// before anything can fail and want to report itself.
	sink := runlog.Open()
	if sink != nil {
		defer sink.Close()

		log.SetOutput(io.MultiWriter(os.Stderr, sink))
		log.SetFlags(log.Ldate | log.Ltime) // a file outlives the day a time-only stamp assumes
	}

	// The triggers fire every few minutes and almost always find nothing to do,
	// so the progress chatter is for humans watching a live run only. An outcome
	// line is different: where there is a log file it is the only evidence the
	// run happened at all, so every run leaves exactly one.
	//
	// stderr, not stdout — that is where the log package writes, so that is the
	// stream whose reader decides whether anyone is watching.
	interactive := term.IsTerminal(int(os.Stderr.Fd()))
	verbose := interactive || sink != nil

	ssid, err := wifi.SSID()
	if err != nil {
		log.Fatalf("could not determine the current network: %v", err)
	}
	if !wifi.IsBITS(ssid) {
		if verbose {
			log.Printf("current WiFi is %q; not a BITS network. Skipping.", ssid)
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

// watch camps on a session about to expire. The portal drops us on a fixed
// timer, so it is the one network event that can be seen coming.
//
// Best-effort throughout: missing state, a lock held elsewhere or a deadline
// that never arrives all just return, and the reactive path handles it.
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

	for time.Now().Before(giveUp) {
		time.Sleep(session.PollInterval)

		if p.IsLoggedIn() {
			continue
		}

		authenticate(p, "")

		return
	}

	log.Print("expiry did not arrive within the grace period, leaving it to the periodic check.")
}

func authenticate(p *portal.Portal, ssid string) {
	c, err := creds.Load(creds.DefaultPath())
	if err != nil {
		log.Fatal(err)
	}

	if ssid != "" {
		log.Printf("not logged in. Authenticating to %s...", ssid)
	}

	// How far the portal's clock ran from ours. Logged here rather than in watch()
	// so the reactive path is measured too — that is the path that runs when the
	// watcher failed to arm, which is the case most worth knowing about.
	if prev := session.Load(session.DefaultPath()); !prev.LoginAt.IsZero() {
		log.Printf("expiry noticed %s after the predicted deadline of %s",
			time.Since(prev.Deadline()).Round(time.Second),
			prev.Deadline().Format(time.TimeOnly))
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		log.Printf("attempt %d/%d...", attempt, attempts)

		s, err := p.Login(c)
		if errors.Is(err, portal.ErrAlreadyOnline) {
			// The outage healed by itself between the two probes. Nothing was
			// wrong and nothing needs doing, so do not burn the remaining
			// attempts and do not exit non-zero over it.
			log.Print("connectivity came back on its own, nothing to do.")

			return
		}
		if err != nil {
			log.Print(err)
		} else if settled(p) {
			saved := session.Observe(session.Load(session.DefaultPath()), s.LoginAt)

			if saved.Timeout > 0 {
				log.Printf("login successful. Sessions last about %s, so the next expiry is due %s",
					saved.Timeout.Round(time.Minute), saved.Deadline().Format(time.DateTime))
			} else {
				log.Print("login successful. No session lifetime measured yet, so the next expiry cannot be anticipated.")
			}

			if err := session.Save(session.DefaultPath(), saved); err != nil {
				log.Printf("could not record the session deadline: %v", err)
			}

			return
		} else {
			log.Print("portal accepted the login but there is still no connectivity.")
		}

		if attempt < attempts {
			time.Sleep(retryDelay << (attempt - 1))
		}
	}

	log.Fatal("all attempts failed.")
}

// settled waits for the session to actually come up, returning the moment it
// does rather than always paying the worst case.
func settled(p *portal.Portal) bool {
	for waited := time.Duration(0); waited < settleTimeout; waited += settleStep {
		time.Sleep(settleStep)

		if p.IsLoggedIn() {
			return true
		}
	}

	return false
}

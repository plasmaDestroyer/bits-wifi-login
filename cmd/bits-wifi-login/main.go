// Command bits-wifi-login authenticates against the BITS Pilani Fortinet captive
// portal. With no arguments it is meant to be fired by a background trigger
// (systemd timer, launchd agent, scheduled task) and exits 0 whenever there is
// nothing to do. `install` and `uninstall` manage those triggers.
package main

import (
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
	"github.com/plasmaDestroyer/bits-wifi-login/internal/wifi"
)

const (
	attempts    = 2
	settleDelay = 2 * time.Second // portal needs a moment before the session is live
	retryDelay  = 3 * time.Second
)

func main() {
	log.SetFlags(log.Ltime)

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
		return
	}

	c, err := creds.Load(creds.DefaultPath())
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("not logged in. Authenticating to %s...", ssid)

	for attempt := 1; attempt <= attempts; attempt++ {
		log.Printf("attempt %d/%d...", attempt, attempts)

		if err := p.Login(c); err != nil {
			log.Print(err)
		} else {
			time.Sleep(settleDelay)
			if p.IsLoggedIn() {
				log.Print("login successful.")
				return
			}
			log.Print("portal accepted the login but there is still no connectivity.")
		}

		if attempt < attempts {
			time.Sleep(retryDelay)
		}
	}

	log.Fatal("all attempts failed.")
}

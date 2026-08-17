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
`)
}

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
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

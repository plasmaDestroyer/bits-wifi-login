// Command bits-wifi-login authenticates against the BITS Pilani Fortinet captive
// portal. It is meant to be fired by a background trigger (systemd timer, launchd
// agent, scheduled task) and exits 0 whenever there is nothing to do.
package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/creds"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/portal"
	"github.com/plasmaDestroyer/bits-wifi-login/internal/wifi"
)

const (
	attempts    = 2
	settleDelay = 2 * time.Second // portal needs a moment before the session is live
	retryDelay  = 3 * time.Second
)

func main() {
	log.SetFlags(log.Ltime)

	ssid, err := wifi.SSID()
	if err != nil {
		log.Fatalf("could not determine the current network: %v", err)
	}
	if !wifi.IsBITS(ssid) {
		log.Printf("current WiFi is %q; not a BITS network. Skipping.", ssid)
		return
	}

	p := portal.New()

	log.Print("checking connectivity...")
	if p.IsLoggedIn() {
		log.Print("already authenticated, nothing to do.")
		return
	}

	c, err := creds.Load(credsPath())
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

// credsPath resolves creds.conf next to the binary. The installers bake absolute
// paths into their triggers and drop creds.conf alongside the program; a trigger
// has no useful working directory to resolve against.
func credsPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "creds.conf"
	}

	return filepath.Join(filepath.Dir(exe), "creds.conf")
}

//go:build linux

package installer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/plasmaDestroyer/bits-wifi-login/internal/portal"
)

// sudoWrite is the Go spelling of `sudo tee <path>`, which is how the shell
// installer wrote into /etc without the whole program running as root —
// creds.conf must stay owned by the user who actually logs in.
func sudoWrite(path, content, mode string) error {
	cmd := exec.Command("sudo", "tee", path)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}

	return run("sudo", "chmod", mode, path)
}

const (
	dispatcherPath   = "/etc/NetworkManager/dispatcher.d/90-fortinet-login"
	connectivityPath = "/etc/NetworkManager/conf.d/99-bits-wifi-login-connectivity.conf"
	servicePath      = "/etc/systemd/system/bits-wifi-login.service"
	timerPath        = "/etc/systemd/system/bits-wifi-login.timer"
	resumePath       = "/etc/systemd/system/bits-wifi-login-resume.service"
)

var units = []string{
	"bits-wifi-login.timer",
	"bits-wifi-login.service",
	"bits-wifi-login-resume.service",
}

// The dispatcher script runs as root and embeds this path in a `su -c` string,
// and systemd splits ExecStart on whitespace. Both make an unusual path a real
// problem, so refuse it up front with an actionable message.
var unsafePath = regexp.MustCompile("[\\s\"'`$\\\\;&|<>()\n]")

func preflight(exe string) error {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return errors.New("installer: NetworkManager (nmcli) not found — is this an NM-managed system?")
	}

	if os.Geteuid() == 0 {
		return errors.New("installer: do not run this with sudo — it will ask for your password when it needs root, and creds.conf must stay owned by you")
	}

	if unsafePath.MatchString(exe) {
		return fmt.Errorf("installer: refusing to install from %q — the path must not contain spaces or shell metacharacters. Move the binary somewhere plainer and re-run", exe)
	}

	return nil
}

func install(exe string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("installer: cannot determine the current user: %w", err)
	}

	// The dispatcher's output used to go to a predictable /tmp path, which any
	// local user could pre-create as a symlink. Keep it under the user's own
	// state dir instead; the systemd units are covered by journald already.
	logDir := filepath.Join(u.HomeDir, ".local", "state", "bits-wifi-login")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("installer: creating %s: %w", logDir, err)
	}
	logPath := filepath.Join(logDir, "dispatcher.log")

	if unsafePath.MatchString(logPath) {
		return fmt.Errorf("installer: refusing to use log path %q — it must not contain spaces or shell metacharacters", logPath)
	}

	fmt.Println("Installing background triggers (sudo may prompt)...")

	// Prime the sudo timestamp before touching /etc. sudoWrite pipes file content
	// on stdin, so sudo must fall back to /dev/tty for the password — without a
	// terminal it fails, and doing that check here means it fails before the first
	// write rather than halfway through, leaving a partial install.
	if err := run("sudo", "-v"); err != nil {
		return errors.New("installer: sudo could not authenticate — run this straight from a terminal (it cannot prompt for a password through a pipe or an editor console)")
	}

	if err := sudoWrite(dispatcherPath, dispatcher(exe, u.Username, logPath), "0755"); err != nil {
		return err
	}
	fmt.Println("✓ NetworkManager dispatcher installed.")

	if err := sudoWrite(connectivityPath, connectivityConf, "0644"); err != nil {
		return err
	}
	if err := run("sudo", "systemctl", "reload", "NetworkManager"); err != nil {
		fmt.Println("⚠ Could not reload NetworkManager — connectivity checking starts at next restart.")
	}
	calibrateConnectivity()

	if err := sudoWrite(resumePath, resumeUnit(exe, u.Username), "0644"); err != nil {
		return err
	}
	fmt.Println("✓ Resume service installed.")

	if err := sudoWrite(servicePath, serviceUnit(exe, u.Username), "0644"); err != nil {
		return err
	}
	fmt.Println("✓ systemd service installed.")

	if err := sudoWrite(timerPath, timerUnit, "0644"); err != nil {
		return err
	}
	fmt.Println("✓ systemd timer installed.")

	if err := run("sudo", "systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("sudo", "systemctl", "enable", "bits-wifi-login-resume.service"); err != nil {
		return err
	}
	if err := run("sudo", "systemctl", "enable", "bits-wifi-login.timer"); err != nil {
		return err
	}

	// restart, not `enable --now`. On a re-install the timer is normally already
	// active, and `start` is a no-op on an active unit — so it would keep its old
	// in-memory schedule and any change to the timer file would silently not
	// apply. Worse, a timer parked in `active (elapsed)` stays parked: only a
	// restart recomputes the next elapse. This is what makes install a repair.
	if err := run("sudo", "systemctl", "restart", "bits-wifi-login.timer"); err != nil {
		return err
	}
	fmt.Println("✓ Timer enabled and started.")

	return nil
}

// NM's connectivity check is worth having only if NM's own probe can reach the
// internet, and under a VPN it frequently cannot. Confirmed 2026-08-28 with
// CloudflareWARP: curl got a clean 204 over both IPv4 and IPv6 while NM's check
// returned "limited". Forcing it on there is worse than leaving it off — NM
// reports a working network as broken to every app on the desktop, and, stuck
// in one wrong state, it can never emit the connectivity-change *transition*
// that this whole trigger depends on.
//
// So do not assume either way. Turn it on, ask NM what it sees, and keep it
// only if NM agrees with a probe that just succeeded. A VPN switched on later
// invalidates the answer, which is why triggers() re-reads NM's live verdict
// and why re-running install is the documented repair.
func calibrateConnectivity() {
	if !portal.New().IsLoggedIn() {
		fmt.Println("⚠ Not online, so NM's connectivity check could not be verified — left as it was.")
		return
	}

	if err := setConnectivityCheck(true); err != nil {
		fmt.Println("⚠ Could not enable NM connectivity checking — the periodic timer is the only trigger.")
		return
	}

	state, _ := runOut("nmcli", "networking", "connectivity", "check")
	if strings.TrimSpace(state) == "full" {
		fmt.Println("✓ NetworkManager connectivity checking enabled and verified.")
		return
	}

	if err := setConnectivityCheck(false); err != nil {
		fmt.Println("⚠ NM's connectivity check disagrees with the network and could not be turned back off.")
		return
	}

	fmt.Printf("⚠ NM reports %q while the network is working — a VPN is most likely\n"+
		"  intercepting NM's probe. Left off, so the periodic timer is the trigger.\n"+
		"  Re-run `bits-wifi-login install` if you change your VPN setup.\n",
		strings.TrimSpace(state))
}

func setConnectivityCheck(on bool) error {
	return run("sudo", "busctl", "set-property",
		"org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager",
		"org.freedesktop.NetworkManager", "ConnectivityCheckEnabled",
		"b", strconv.FormatBool(on))
}

func uninstall() (int, error) {
	removed := 0

	// Disable unconditionally so stale symlinks go too; a unit that was never
	// installed just reports a failure we can ignore. run, not runOut, all the
	// way down here: sudo may still need a terminal to prompt on, and swallowing
	// its stderr would swallow the password prompt with it.
	for _, unit := range units {
		if err := run("sudo", "systemctl", "disable", "--now", unit); err != nil {
			fmt.Printf("• Not found or already removed: %s\n", unit)
		} else {
			removed++
			fmt.Printf("✓ Disabled and stopped %s\n", unit)
		}
	}

	for _, path := range []string{timerPath, servicePath, resumePath, dispatcherPath, connectivityPath} {
		// rm -f cannot tell us whether there was anything there, and reporting a
		// removal that did not happen is how uninstall ends up claiming it cleaned
		// a machine it never touched. Everything here is world-readable, so the
		// existence check needs no root of its own.
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("• Not found: %s\n", path)
			continue
		}

		if err := run("sudo", "rm", "-f", path); err != nil {
			return removed, err
		}

		removed++
		fmt.Printf("✓ Removed %s\n", path)
	}

	// Nothing was there, so there is nothing to reload — and no reason to make a
	// second uninstall ask for a root password to do it.
	if removed == 0 {
		return 0, nil
	}

	return removed, run("sudo", "systemctl", "daemon-reload")
}

// is-enabled and a stat, not systemctl status: neither needs root, and status
// would report "inactive" for a oneshot service that is working perfectly.
//
// is-enabled exits non-zero for "disabled", which is the *correct* state for
// bits-wifi-login.service — install only enables the timer and the resume unit,
// and the timer is what pulls the service in. Judge on the word, not the exit
// status, or a healthy install reports its own service missing.
func triggers() []Trigger {
	found := make([]Trigger, 0, len(units)+2)

	for _, unit := range units {
		state, err := runOut("systemctl", "is-enabled", unit)
		registered := err == nil || strings.TrimSpace(state) == "disabled"
		found = append(found, Trigger{Name: unit, Registered: registered})
	}

	for _, path := range []string{dispatcherPath, connectivityPath} {
		_, err := os.Stat(path)
		found = append(found, Trigger{Name: filepath.Base(path), Registered: err == nil})
	}

	// The connectivity file existing proves nothing — NM can be told at runtime
	// to stop checking, and then it reads the file, reports its uri back, and
	// never probes. That is invisible from the filesystem and it is what turned
	// connectivity-change into a dead trigger for weeks. Ask NM itself.
	// Naming NM's live verdict matters as much as the on/off bit: a VPN turned on
	// after install leaves this enabled but wrong, and an NM stuck in one wrong
	// state emits no transition and so fires nothing.
	name := "NM connectivity checking (the connectivity-change trigger)"
	if verdict, err := runOut("nmcli", "networking", "connectivity"); err == nil {
		name = fmt.Sprintf("%s — NM currently reports %q", name, strings.TrimSpace(verdict))
	}

	found = append(found, Trigger{Name: name, Registered: connectivityCheckEnabled()})

	return found
}

func connectivityCheckEnabled() bool {
	out, err := runOut("busctl", "get-property",
		"org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager",
		"org.freedesktop.NetworkManager", "ConnectivityCheckEnabled")

	return err == nil && strings.TrimSpace(out) == "b true"
}

func summary() string {
	return "  Triggers:\n" +
		"    - Every WiFi connect to a BITS network (NetworkManager dispatcher)\n" +
		"    - The moment NM notices connectivity dropped (connectivity-change)\n" +
		"    - Every resume from suspend/sleep (systemd resume service)\n" +
		"    - Every 10 minutes (systemd timer, persistent across sleep)\n\n" +
		"  Logs:\n" +
		"    journalctl -u bits-wifi-login.service --since today\n" +
		"    tail ~/.local/state/bits-wifi-login/dispatcher.log\n\n" +
		"  Repair:\n" +
		"    Re-run `bits-wifi-login install` if triggers or permissions break.\n"
}

// The dispatcher waits for the network to actually answer before logging in —
// NM reports "up" well before the captive portal is reachable.
//
// `connectivity-change` is what closes the silent-drop gap: NM's own periodic
// connectivity probe flips to "portal" the moment the session dies, and that
// fires here immediately instead of waiting for the next timer tick.
//
// The readiness check is "did anything answer at all", i.e. curl's exit status,
// NOT the HTTP status. It used to wait for a 204 or a 302, which is exactly
// backwards: an intercepted probe answers 200 with the fgtauth page in the body
// (no Location header — see internal/portal), so in the one situation this hook
// exists for, the loop could never match. It burned its ten tries and exited
// without logging in, which is the 30-60s of dead network after a session
// expiry. 204 means already online and 302 was never observed.
func dispatcher(exe, username, logPath string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
CURRENT_SSID=$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep '^yes' | cut -d: -f2)
if [[ "$2" == "up" || "$2" == "connectivity-change" ]] && [[ "$CURRENT_SSID" =~ ^BITS-(STUDENT|STAFF)$ ]]; then
    tries=0
    until curl -s --max-time 3 -o /dev/null \
        "http://connectivitycheck.gstatic.com/generate_204"; do
        tries=$((tries + 1))
        [[ $tries -ge 10 ]] && exit 0
        sleep 2
    done
    su -c "%s >> %s 2>&1" %s
fi
`, exe, logPath, username)
}

// TimeoutStartSec=0 because a run that lands near the portal's expiry camps on it
// and polls, which takes minutes. Type=oneshot already defaults to no start
// timeout, so this is stated rather than needed — it stops a distro-level
// DefaultTimeoutStartSec or a later change of Type from silently capping the
// watcher, which would look like the feature was installed and simply never
// firing. Windows had the same hazard for real: its task XML capped runs at PT2M.
func serviceUnit(exe, username string) string {
	return fmt.Sprintf(`[Unit]
Description=BITS WiFi Fortinet Login
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=%s
ExecStart=%s
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
`, username, exe)
}

// ExecStartPre gives the Wi-Fi radio time to reassociate after a resume;
// without it the login fires before there is any SSID to detect.
func resumeUnit(exe, username string) string {
	return fmt.Sprintf(`[Unit]
Description=BITS WiFi Login after resume
After=suspend.target hibernate.target hybrid-sleep.target suspend-then-hibernate.target

[Service]
Type=oneshot
User=%s
ExecStartPre=/bin/bash -c '\
    wifi_status=$(nmcli radio wifi 2>/dev/null); \
    [[ "$wifi_status" != "enabled" ]] && exit 0; \
    tries=0; \
    while [[ -z "$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep ^yes | cut -d: -f2)" ]]; do \
        tries=$((tries+1)); \
        [[ $tries -ge 15 ]] && exit 0; \
        sleep 2; \
    done'
ExecStart=%s

[Install]
WantedBy=suspend.target hibernate.target hybrid-sleep.target suspend-then-hibernate.target
`, username, exe)
}

// The portal session expires on a fixed ~4h boundary, so a bare polling timer
// aliases against it: a tick landing just before expiry no-ops and the next one
// is a whole interval away. connectivity-change is what actually closes that,
// which leaves this as a fallback for hosts where NM connectivity checking is
// off — 10 minutes, not the 30 that produced the long waits.
// OnCalendar, not OnUnitActiveSec: the latter re-arms only against the last
// *service* activation, so if the service has not run for a while and the timer
// is then restarted, every trigger evaluates into the past and the unit parks in
// `active (elapsed)` with `Trigger: n/a` — dead forever, silently. A wall-clock
// expression always has a next occurrence. Check with `systemctl list-timers`:
// a healthy timer shows a NEXT, a broken one shows `-`.
const timerUnit = `[Unit]
Description=BITS WiFi Login periodic check

[Timer]
OnBootSec=30s
OnCalendar=*:0/10
Persistent=true

[Install]
WantedBy=timers.target
`

// NM only emits connectivity-change if its own connectivity checking is on, and
// several distros ship it disabled — but `enabled` is deliberately NOT set here.
// A runtime override lives in /var/lib/NetworkManager/NetworkManager-intern.conf,
// which NM reads after conf.d and which therefore wins; setting the key in both
// places just makes it ambiguous which one is in force. calibrateConnectivity
// drives the D-Bus property instead, so there is exactly one lever.
//
// interval is the detection budget once it is on: nothing notices an expired
// session until NM's next probe, so at NM's 300s default the session could be
// dead for five minutes with the machine sitting there. 20s costs one HTTP
// request to gstatic every 20s — nothing next to a Wi-Fi link that is already
// idling — and bounds the visible outage at roughly 20s plus the login round
// trips.
const connectivityConf = `[connectivity]
uri=http://connectivitycheck.gstatic.com/generate_204
interval=20
`

//go:build linux

package installer

import (
	"context"
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
	"time"

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

	// Not /tmp: a predictable path there is a symlink-attack target.
	logDir := filepath.Join(u.HomeDir, ".local", "state", "bits-wifi-login")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("installer: creating %s: %w", logDir, err)
	}
	logPath := filepath.Join(logDir, "dispatcher.log")

	if unsafePath.MatchString(logPath) {
		return fmt.Errorf("installer: refusing to use log path %q — it must not contain spaces or shell metacharacters", logPath)
	}

	fmt.Println("Installing background triggers (sudo may prompt)...")

	// Prime sudo before touching /etc: sudoWrite pipes content on stdin, so sudo
	// needs /dev/tty for the password. Failing here beats a partial install.
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

// Under a VPN, NM's own probe often cannot get out: confirmed 2026-08-28, curl
// got a clean 204 while NM's check said "limited". Forcing it on there is worse
// than off — NM reports a working network as broken to every app, and, stuck in
// one wrong state, can never emit the *transition* this trigger needs.
//
// So do not assume: enable, ask NM what it sees, keep it only if NM agrees with
// a probe that just succeeded. A VPN enabled later invalidates that, which is
// why triggers() re-reads the live verdict and re-running install is the repair.
func calibrateConnectivity() {
	if !portal.New().IsLoggedIn() {
		fmt.Println("⚠ Not online, so NM's connectivity check could not be verified — left as it was.")
		return
	}

	if err := setConnectivityCheck(true); err != nil {
		fmt.Println("⚠ Could not enable NM connectivity checking — the periodic timer is the only trigger.")
		return
	}

	// Bounded, because `connectivity check` forces NM to probe for real and a
	// broken probe blocks until NM's own timeout — ~10s of an install spent
	// waiting for an answer we are about to reject anyway.
	//
	// Slow IS the answer here. This trigger exists to notice a drop within one
	// 20s interval; a check that cannot finish in a few seconds is useless for
	// that even when it eventually says "full".
	state, err := nmConnectivity(checkTimeout)
	if err != nil {
		_ = setConnectivityCheck(false)
		fmt.Printf("⚠ NM's connectivity check did not answer within %s, so it is too slow to\n"+
			"  detect a drop. Left off, so the periodic timer is the trigger.\n", checkTimeout)

		return
	}

	if state == "full" {
		fmt.Println("✓ NetworkManager connectivity checking enabled and verified.")
		return
	}

	if err := setConnectivityCheck(false); err != nil {
		fmt.Println("⚠ NM's connectivity check disagrees with the network and could not be turned back off.")
		return
	}

	fmt.Printf("⚠ NM reports %q while the network is working — a VPN is most likely\n"+
		"  intercepting NM's probe. Left off, so the periodic timer is the trigger.\n"+
		"  Re-run `bits-wifi-login install` if you change your VPN setup.\n", state)
}

// checkTimeout is generous for a probe that a working network answers in well
// under a second, and short enough that a broken one does not stall the install.
const checkTimeout = 4 * time.Second

func nmConnectivity(limit time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nmcli", "networking", "connectivity", "check")
	// Killing the process is not enough to unblock Output(): anything that
	// inherited its stdout keeps the pipe open and the read waits on that, not on
	// the process. WaitDelay closes the pipes shortly after the kill so this
	// cannot outlive its deadline no matter what nmcli left behind.
	cmd.WaitDelay = time.Second

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
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
	// The live verdict matters as much as the on/off bit: a VPN enabled after
	// install leaves this on but wrong, and a stuck NM fires nothing.
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

// TimeoutStartSec=0 because a run near the expiry camps on it for minutes.
// oneshot already defaults to no timeout; stating it stops a distro-level
// DefaultTimeoutStartSec from silently capping the watcher.
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

// A fallback only: the session expires on a fixed wall-clock instant, which a
// bare poll aliases against — measured 2026-08-29, this tick fired 12s before
// the drop and the next chance was 10 minutes later. The watcher is what
// actually catches expiries.
//
// OnCalendar, not OnUnitActiveSec: the latter re-arms against the last *service*
// activation, so a restart can evaluate every trigger into the past and park the
// unit in `active (elapsed)` forever. `systemctl list-timers` shows NEXT as `-`.
const timerUnit = `[Unit]
Description=BITS WiFi Login periodic check

[Timer]
OnBootSec=30s
OnCalendar=*:0/10
Persistent=true

[Install]
WantedBy=timers.target
`

// `enabled` is deliberately NOT set here: a runtime override in
// /var/lib/NetworkManager/NetworkManager-intern.conf is read after conf.d and
// wins, so calibrateConnectivity drives the D-Bus property and there is exactly
// one lever. interval is the detection budget once it is on; NM's 300s default
// would leave a dead session for five minutes.
const connectivityConf = `[connectivity]
uri=http://connectivitycheck.gstatic.com/generate_204
interval=20
`

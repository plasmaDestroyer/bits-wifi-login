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

// sudoWrite writes into /etc without running the whole program as root, so
// creds.conf stays user-owned. `install -m`, not tee+chmod: one sudo per file
// and never a moment with the wrong mode.
func sudoWrite(path, content, mode string) error {
	cmd := exec.Command("sudo", "install", "-m", mode, "/dev/stdin", path)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer: writing %s: %w", path, err)
	}

	return nil
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

// The path lands in a root-run `su -c` string and in ExecStart, which systemd
// splits on whitespace. Refuse odd paths instead of quoting for both.
var unsafePath = regexp.MustCompile("[\\s\"'`$\\\\;&|<>()\n]")

func preflight(exe string) error {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return errors.New("installer: NetworkManager (nmcli) not found. Is this an NM-managed system?")
	}

	if os.Geteuid() == 0 {
		return errors.New("installer: do not run this with sudo, it asks for root itself")
	}

	if unsafePath.MatchString(exe) {
		return fmt.Errorf("installer: refusing to install from %q: the path must not contain spaces or shell metacharacters", exe)
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
		return fmt.Errorf("installer: refusing to use log path %q: it must not contain spaces or shell metacharacters", logPath)
	}

	fmt.Println("Installing background triggers (sudo may prompt)...")

	// Prime sudo first: sudoWrite uses stdin for content, so the password needs
	// /dev/tty. Failing here beats a half install.
	if err := run("sudo", "-v"); err != nil {
		return errors.New("installer: sudo could not authenticate. Run this from a terminal")
	}

	if err := sudoWrite(dispatcherPath, dispatcher(exe, u.Username, logPath), "0755"); err != nil {
		return err
	}
	fmt.Println("✓ NetworkManager dispatcher installed.")

	if err := sudoWrite(connectivityPath, connectivityConf, "0644"); err != nil {
		return err
	}
	if err := run("sudo", "systemctl", "reload", "NetworkManager"); err != nil {
		fmt.Println("⚠ Could not reload NetworkManager. Connectivity checking starts at its next restart.")
	}
	// NM's probe can block for seconds. Let it run while the units go in.
	verdict := make(chan string, 1)
	go func() { verdict <- calibrateConnectivity() }()

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

	// restart, NOT `enable --now`: `start` is a no-op on an active timer, so it
	// keeps its old schedule, and one parked in `active (elapsed)` stays parked.
	if err := run("sudo", "systemctl", "restart", "bits-wifi-login.timer"); err != nil {
		return err
	}
	fmt.Println("✓ Timer enabled and started.")

	// Only spin if NM is still thinking.
	select {
	case line := <-verdict:
		fmt.Println(line)
	default:
		spin("Waiting for NetworkManager's connectivity check...").done(<-verdict)
	}

	return nil
}

// Under a VPN, NM's own probe often can't get out (2026-08-28): NM then calls a
// working network broken and, stuck there, never fires the transition this
// trigger needs. So enable, ask NM, and keep it only if NM agrees with a probe
// that just worked. Returns its line instead of printing: it runs concurrently.
func calibrateConnectivity() string {
	if !portal.New().IsLoggedIn() {
		return "⚠ Not online, so NM's connectivity check could not be verified. Left as it was."
	}

	if err := setConnectivityCheck(true); err != nil {
		return "⚠ Could not enable NM connectivity checking. The periodic timer is the only trigger."
	}

	// Bounded: a broken probe blocks ~10s. Slow IS the answer, too: a check
	// that can't finish in seconds is useless for 20s drop detection.
	state, err := nmConnectivity(checkTimeout)
	if err != nil {
		_ = setConnectivityCheck(false)

		return fmt.Sprintf("⚠ NM's connectivity check did not answer within %s, so it is too slow\n"+
			"  to detect a drop. Left off, so the periodic timer is the trigger.", checkTimeout)
	}

	if state == "full" {
		return "✓ NetworkManager connectivity checking enabled and verified."
	}

	if err := setConnectivityCheck(false); err != nil {
		return "⚠ NM's connectivity check disagrees with the network and could not be turned back off."
	}

	return fmt.Sprintf("⚠ NM reports %q while the network is working. A VPN is most likely\n"+
		"  intercepting NM's probe. Left off, so the periodic timer is the trigger.\n"+
		"  Re-run `bits-wifi-login install` if you change your VPN setup.", state)
}

const checkTimeout = 4 * time.Second

func nmConnectivity(limit time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nmcli", "networking", "connectivity", "check")
	// Killing nmcli doesn't unblock Output() if a child kept the pipe open.
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

	// Disable unconditionally so stale symlinks go too. run, not runOut: don't
	// swallow sudo's password prompt.
	for _, unit := range units {
		if err := run("sudo", "systemctl", "disable", "--now", unit); err != nil {
			fmt.Printf("• Not found or already removed: %s\n", unit)
		} else {
			removed++
			fmt.Printf("✓ Disabled and stopped %s\n", unit)
		}
	}

	for _, path := range []string{timerPath, servicePath, resumePath, dispatcherPath, connectivityPath} {
		// rm -f can't say whether anything was there. Don't claim removals.
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

	if removed == 0 {
		return 0, nil
	}

	return removed, run("sudo", "systemctl", "daemon-reload")
}

// is-enabled, not status: a healthy oneshot reads "inactive". And judge the
// word, not the exit code: "disabled" is correct for the timer-driven service.
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

	// Ask NM itself: the conf file can exist while checking is off at runtime,
	// and a VPN added after install leaves it on but stuck.
	name := "NM connectivity checking (the connectivity-change trigger)"
	if verdict, err := runOut("nmcli", "networking", "connectivity"); err == nil {
		name = fmt.Sprintf("%s (NM currently reports %q)", name, strings.TrimSpace(verdict))
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

// connectivity-change is listed only if calibration kept it on.
func summary() string {
	drop := "    - The moment NM notices connectivity dropped (connectivity-change)\n"
	timer := "    - Every 10 minutes (systemd timer, persistent across sleep)\n"

	if !connectivityCheckEnabled() {
		drop = ""
		timer = "    - Every 10 minutes (systemd timer). This is the only drop detection on this\n" +
			"      machine, since NM's connectivity check is off\n"
	}

	return "  Triggers:\n" +
		"    - Every WiFi connect to a BITS network (NetworkManager dispatcher)\n" +
		drop +
		"    - Every resume from suspend/sleep (systemd resume service)\n" +
		timer + "\n" +
		"  Logs:\n" +
		"    journalctl -u bits-wifi-login.service --since today\n" +
		"    tail ~/.local/state/bits-wifi-login/dispatcher.log\n"
}

// NM says "up" before the portal answers, so the dispatcher waits first.
// `connectivity-change` fires the moment NM's probe flips to "portal".
//
// Readiness = curl's exit status, NOT the HTTP status: an intercepted probe
// answers 200 with the fgtauth page, so waiting for 204/302 never matched in
// the one state worth logging in from.
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

// TimeoutStartSec=0: the watcher camps for minutes, and a distro
// DefaultTimeoutStartSec would cut it off.
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

// ExecStartPre: let Wi-Fi reassociate after resume, or there's no SSID yet.
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

// Fallback only. The watcher catches expiries. OnCalendar, NOT
// OnUnitActiveSec: that one re-arms off the last service run and can park the
// unit in `active (elapsed)` forever (list-timers shows NEXT as `-`).
const timerUnit = `[Unit]
Description=BITS WiFi Login periodic check

[Timer]
OnBootSec=30s
OnCalendar=*:0/10
Persistent=true

[Install]
WantedBy=timers.target
`

// No `enabled` here: NM's runtime override wins over conf.d, so
// calibrateConnectivity flips it over D-Bus instead. interval=20 is the whole
// detection budget (NM's default 300s = five dead minutes).
//
// `response` MUST be set whenever `uri` is. conf.d merges per key, so the
// distro's expected text (Arch: "NetworkManager is online") survives, NM calls
// generate_204's empty 204 a "portal", and calibration kills the trigger.
const connectivityConf = `[connectivity]
uri=http://connectivitycheck.gstatic.com/generate_204
response=
interval=20
`

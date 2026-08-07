//go:build linux

package installer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
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
	dispatcherPath = "/etc/NetworkManager/dispatcher.d/90-fortinet-login"
	servicePath    = "/etc/systemd/system/bits-wifi-login.service"
	timerPath      = "/etc/systemd/system/bits-wifi-login.timer"
	resumePath     = "/etc/systemd/system/bits-wifi-login-resume.service"
)

var units = []string{
	"bits-wifi-login.timer",
	"bits-wifi-login.service",
	"bits-wifi-login-resume.service",
}

func preflight() error {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return errors.New("installer: NetworkManager (nmcli) not found — is this an NM-managed system?")
	}

	if os.Geteuid() == 0 {
		return errors.New("installer: do not run this with sudo — it will ask for your password when it needs root, and creds.conf must stay owned by you")
	}

	return nil
}

func install(exe string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("installer: cannot determine the current user: %w", err)
	}

	fmt.Println("Installing background triggers (sudo may prompt)...")

	if err := sudoWrite(dispatcherPath, dispatcher(exe, u.Username), "0755"); err != nil {
		return err
	}
	fmt.Println("✓ NetworkManager dispatcher installed.")

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
	if err := run("sudo", "systemctl", "enable", "--now", "bits-wifi-login.timer"); err != nil {
		return err
	}
	fmt.Println("✓ Timer enabled and started.")

	return nil
}

func uninstall() error {
	// Disable unconditionally so stale symlinks go too; a unit that was never
	// installed just reports a failure we can ignore.
	for _, unit := range units {
		if err := run("sudo", "systemctl", "disable", "--now", unit); err != nil {
			fmt.Printf("⚠ Not found or already removed: %s\n", unit)
		} else {
			fmt.Printf("✓ Disabled and stopped %s\n", unit)
		}
	}

	for _, path := range []string{timerPath, servicePath, resumePath, dispatcherPath} {
		if err := run("sudo", "rm", "-f", path); err != nil {
			return err
		}
		fmt.Printf("✓ Removed %s\n", path)
	}

	return run("sudo", "systemctl", "daemon-reload")
}

func summary() string {
	return "  Triggers:\n" +
		"    - Every WiFi connect to a BITS network (NetworkManager dispatcher)\n" +
		"    - Every resume from suspend/sleep (systemd resume service)\n" +
		"    - Every 30 minutes (systemd timer, persistent across sleep)\n\n" +
		"  Logs:\n" +
		"    journalctl -u bits-wifi-login.service --since today\n\n" +
		"  Repair:\n" +
		"    Re-run `bits-wifi-login install` if triggers or permissions break.\n"
}

// The dispatcher waits for the network to actually answer before logging in —
// NM reports "up" well before the captive portal is reachable.
func dispatcher(exe, username string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
CURRENT_SSID=$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep '^yes' | cut -d: -f2)
if [[ "$2" == "up" ]] && [[ "$CURRENT_SSID" =~ ^BITS-(STUDENT|STAFF)$ ]]; then
    tries=0
    until curl -s --max-time 3 -o /dev/null -w "%%{http_code}" \
        "http://connectivitycheck.gstatic.com/generate_204" | grep -q "204\|302"; do
        tries=$((tries + 1))
        [[ $tries -ge 10 ]] && exit 0
        sleep 3
    done
    su -c "%s >> /tmp/bits-wifi-login-%s.log 2>&1" %s
fi
`, exe, username, username)
}

func serviceUnit(exe, username string) string {
	return fmt.Sprintf(`[Unit]
Description=BITS WiFi Fortinet Login
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=%s
ExecStart=%s

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

const timerUnit = `[Unit]
Description=BITS WiFi Login periodic check

[Timer]
OnBootSec=30s
OnUnitActiveSec=30min
Persistent=true

[Install]
WantedBy=timers.target
`

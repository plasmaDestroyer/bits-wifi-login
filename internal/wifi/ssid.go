// Package wifi reports the SSID the machine is currently associated with.
//
// Each platform shells out to the same tool the shell implementation used, and
// the parsing is split from the exec so it can be tested without a radio.
package wifi

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

var bitsRe = regexp.MustCompile(`^BITS-(STUDENT|STAFF)$`)

func IsBITS(ssid string) bool {
	return bitsRe.MatchString(ssid)
}

// SSID returns the current network name, or "" when not associated with any.
func SSID() (string, error) {
	switch runtime.GOOS {
	case "linux":
		out, err := run("nmcli", "-t", "-f", "active,ssid", "dev", "wifi")
		if err != nil {
			return "", err
		}
		return parseNmcli(out), nil

	case "darwin":
		ports, err := run("networksetup", "-listallhardwareports")
		if err != nil {
			return "", err
		}
		iface := parseHardwarePorts(ports)
		if iface == "" {
			return "", nil
		}
		out, err := run("networksetup", "-getairportnetwork", iface)
		if err != nil {
			return "", err
		}
		return parseAirport(out), nil

	case "windows":
		out, err := run("netsh", "wlan", "show", "interfaces")
		if err != nil {
			return "", err
		}
		return parseNetsh(out), nil
	}

	return "", fmt.Errorf("wifi: no SSID backend for %s", runtime.GOOS)
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("wifi: %s: %w", name, err)
	}
	return string(out), nil
}

// parseNmcli reads `nmcli -t -f active,ssid dev wifi`: one "active:ssid" record
// per line, of which exactly one is prefixed "yes:". -t escapes colons inside
// the SSID as "\:".
func parseNmcli(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if ssid, ok := strings.CutPrefix(line, "yes:"); ok {
			return strings.ReplaceAll(strings.TrimSpace(ssid), `\:`, ":")
		}
	}
	return ""
}

// parseHardwarePorts reads `networksetup -listallhardwareports` and returns the
// device of the Wi-Fi port, which is named on the line after the port name.
func parseHardwarePorts(out string) string {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "Wi-Fi") && !strings.Contains(line, "AirPort") {
			continue
		}
		if i+1 >= len(lines) {
			break
		}
		fields := strings.Fields(lines[i+1])
		if len(fields) == 0 {
			break
		}
		return fields[len(fields)-1]
	}
	return ""
}

// parseAirport reads `networksetup -getairportnetwork <iface>`, which prints
// "Current Wi-Fi Network: <ssid>" when associated and a prose sentence when not.
func parseAirport(out string) string {
	name, found := strings.CutPrefix(strings.TrimSpace(out), "Current Wi-Fi Network: ")
	if !found {
		return ""
	}
	return name
}

// netshRe deliberately anchors on "SSID" at the start of the field so the
// adjacent "BSSID" line cannot match.
var netshRe = regexp.MustCompile(`(?m)^\s*SSID\s+:\s*(.*)$`)

func parseNetsh(out string) string {
	m := netshRe.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

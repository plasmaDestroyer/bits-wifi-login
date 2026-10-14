package installer

import "fmt"

// LaunchAgent plist generation. Untagged on purpose so its tests run in CI on
// every platform, not only on a Mac.

const label = "ac.bits.wifi-login"

// 5 min, not Linux's 10: macOS has no connectivity-change, so this poll is the
// only thing that notices an expiry.
func plist(exe string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>

    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>

    <key>WatchPaths</key>
    <array>
        <string>/var/run/resolv.conf</string>
    </array>

    <key>StartInterval</key>
    <integer>300</integer>

    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, label, xmlEscape(exe))
}

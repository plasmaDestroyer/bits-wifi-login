package installer

import "fmt"

// LaunchAgent plist generation. Deliberately NOT build-tagged, even though only
// the darwin build uses it, so its tests run on every platform in CI instead of
// only on a Mac — the same reason windowstask.go is untagged.
//
// The one thing a unit test cannot prove is that launchd accepts the file. That
// is TestPlistIsWellFormed, which shells out to plutil and therefore only runs
// on the macOS CI runner.

const label = "ac.bits.wifi-login"

// 5 minutes rather than Linux's 10: WatchPaths only fires on network changes,
// and macOS has no equivalent of NM's connectivity-change, so this poll is the
// only thing that notices the portal session expiring.
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

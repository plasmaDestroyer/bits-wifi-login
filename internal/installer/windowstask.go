package installer

// Task XML generation. Deliberately NOT build-tagged, even though only the
// Windows installer uses it: it is pure string building, and keeping it
// buildable everywhere means the tests run in CI on Linux instead of only when
// somebody happens to run `go test` on a Windows box.

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	mainTask  = "BITS-WiFi-Login"
	eventTask = "BITS-WiFi-Login-OnConnect"
)

// The two ways a task can carry the user's identity. S4U ("run whether the user
// is logged on or not") runs outside the desktop session, which is what keeps
// the login invisible; InteractiveToken needs a session and therefore gets a
// console window. Only the fallback in install() should ever pick the latter.
const (
	logonS4U         = "S4U"
	logonInteractive = "InteractiveToken"
)

// The periodic trigger cannot start in the past, so give it a minute of slack.
func startBoundary() string {
	return time.Now().Add(time.Minute).Format("2006-01-02T15:04:05")
}

// utf16LE encodes with a BOM, which is what schtasks /xml documents, what every
// Windows build accepts, and what the task document's own declaration says it
// is. Current builds also tolerate UTF-8 — that was measured, not assumed — but
// tolerance is not a contract, and writing what we declare costs six lines.
func utf16LE(s string) []byte {
	encoded := utf16.Encode([]rune(s))

	out := make([]byte, 0, 2+len(encoded)*2)
	out = append(out, 0xFF, 0xFE)
	for _, r := range encoded {
		out = append(out, byte(r), byte(r>>8))
	}

	return out
}

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))

	return b.String()
}

// Run the binary itself, with no shell in the way.
//
// This used to be `cmd.exe /c "<exe> >> <log> 2>&1"`, because a scheduled task
// discards stdout and Windows has no journald to catch it. That wrapper is
// exactly what flashed a console window on screen every five minutes: cmd.exe
// always gets a console, and <Hidden> only hides the task from the Task
// Scheduler list, never the window. The binary keeps its own log now
// (internal/runlog), so the shell has nothing left to do.
//
// <Command> is a path, not a command line, so a path with spaces needs no
// quoting here — the old doubled-quote idiom was a property of cmd, not of the
// task schema.
func action(exe string) string {
	return fmt.Sprintf(`  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
    </Exec>
  </Actions>`, xmlEscape(exe))
}

// ExecutionTimeLimit has to outlast a watch: a run landing near the portal's
// expiry camps on it and polls for up to WatchWindow+WatchGrace. At the old PT2M
// the task registered fine and the watcher was killed a couple of minutes in,
// which is invisible except as an expiry that nothing anticipated.
const taskSettings = `  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>true</RunOnlyIfNetworkAvailable>
    <ExecutionTimeLimit>PT20M</ExecutionTimeLimit>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
  </Settings>`

func principals(user, logon string) string {
	return fmt.Sprintf(`  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>%s</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>`, xmlEscape(user), logon)
}

// 5 minutes rather than Linux's 10: the NetworkProfile event only fires on
// connect, so like macOS this poll is the only thing that notices the portal
// session expiring on its own.
func mainTaskXML(user, exe, logon string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo />
  <Triggers>
    <TimeTrigger>
      <StartBoundary>%s</StartBoundary>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>PT5M</Interval>
        <Duration>P9999D</Duration>
      </Repetition>
    </TimeTrigger>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%s</UserId>
    </LogonTrigger>
  </Triggers>
%s
%s
%s
</Task>
`, startBoundary(), xmlEscape(user), principals(user, logon), taskSettings, action(exe))
}

func eventTaskXML(user, exe, logon string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo />
  <Triggers>
    <EventTrigger>
      <Enabled>true</Enabled>
      <Subscription>&lt;QueryList&gt;&lt;Query Id="0" Path="Microsoft-Windows-NetworkProfile/Operational"&gt;&lt;Select Path="Microsoft-Windows-NetworkProfile/Operational"&gt;*[System[EventID=10000]]&lt;/Select&gt;&lt;/Query&gt;&lt;/QueryList&gt;</Subscription>
      <Delay>PT3S</Delay>
    </EventTrigger>
    <EventTrigger>
      <Enabled>true</Enabled>
      <Subscription>&lt;QueryList&gt;&lt;Query Id="0" Path="System"&gt;&lt;Select Path="System"&gt;*[System[Provider[@Name='Microsoft-Windows-Power-Troubleshooter'] and EventID=1]]&lt;/Select&gt;&lt;/Query&gt;&lt;/QueryList&gt;</Subscription>
      <Delay>PT5S</Delay>
    </EventTrigger>
  </Triggers>
%s
%s
%s
</Task>
`, principals(user, logon), taskSettings, action(exe))
}

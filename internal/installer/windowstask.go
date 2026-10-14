package installer

// Task XML generation. Untagged on purpose so its tests run in CI everywhere.

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

// S4U runs outside the desktop session, so no console window. InteractiveToken
// gets one, and is only install()'s fallback.
const (
	logonS4U         = "S4U"
	logonInteractive = "InteractiveToken"
)

// The periodic trigger cannot start in the past, so give it a minute of slack.
func startBoundary() string {
	return time.Now().Add(time.Minute).Format("2006-01-02T15:04:05")
}

// utf16LE encodes with a BOM: what schtasks /xml documents and what the XML
// declaration claims. UTF-8 works today too, but that is tolerance, not a contract.
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

// Run the binary itself, no shell: a cmd.exe wrapper flashed a console window
// every five minutes (<Hidden> hides the task from the Scheduler list, not the
// window). The binary logs on its own (internal/runlog).
// <Command> is a path, not a command line, so spaces need no quoting.
func action(exe string) string {
	return fmt.Sprintf(`  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
    </Exec>
  </Actions>`, xmlEscape(exe))
}

// ExecutionTimeLimit must outlast WatchWindow+WatchGrace, or Task Scheduler
// quietly kills the watcher mid-camp.
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

// 5 min, not Linux's 10: NetworkProfile only fires on connect, so this poll is
// the only thing that notices an expiry.
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

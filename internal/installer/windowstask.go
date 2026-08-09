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

// The periodic trigger cannot start in the past, so give it a minute of slack.
func startBoundary() string {
	return time.Now().Add(time.Minute).Format("2006-01-02T15:04:05")
}

// utf16LE encodes with a BOM, which is what schtasks expects from /xml — it
// rejects UTF-8, including UTF-8 with a BOM.
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

// The binary logs to stdout and a scheduled task discards that, so route it
// through cmd.exe to keep a log file — Windows has no journald here.
//
// The quoting is the documented `cmd /c "..."` idiom: cmd strips the outermost
// pair, leaving each path individually quoted. That is what makes a path
// containing spaces or an & survive, so don't "simplify" the doubled quotes.
func taskArguments(exe, logFile string) string {
	return fmt.Sprintf(`/c ""%s" >> "%s" 2>&1"`, exe, logFile)
}

func action(exe, logFile string) string {
	return fmt.Sprintf(`  <Actions Context="Author">
    <Exec>
      <Command>cmd.exe</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>`, xmlEscape(taskArguments(exe, logFile)))
}

const taskSettings = `  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>true</RunOnlyIfNetworkAvailable>
    <ExecutionTimeLimit>PT2M</ExecutionTimeLimit>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
  </Settings>`

func principals(user string) string {
	return fmt.Sprintf(`  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>`, xmlEscape(user))
}

// 5 minutes rather than Linux's 10: the NetworkProfile event only fires on
// connect, so like macOS this poll is the only thing that notices the portal
// session expiring on its own.
func mainTaskXML(user, exe, logFile string) string {
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
`, startBoundary(), xmlEscape(user), principals(user), taskSettings, action(exe, logFile))
}

func eventTaskXML(user, exe, logFile string) string {
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
`, principals(user), taskSettings, action(exe, logFile))
}

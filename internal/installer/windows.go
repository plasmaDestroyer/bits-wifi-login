//go:build windows

package installer

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

// The periodic trigger cannot start in the past, so give it a minute of slack.
func startBoundary() string {
	return time.Now().Add(time.Minute).Format("2006-01-02T15:04:05")
}

const (
	mainTask  = "BITS-WiFi-Login"
	eventTask = "BITS-WiFi-Login-OnConnect"
)

// ponytail: no upfront elevation check. schtasks reports "Access is denied."
// clearly enough, and registering a task for the current user often does not
// need elevation at all — checking would only reject cases that work.
//
// The path does get checked: it is embedded in a quoted `cmd.exe /c` string, so
// a quote inside it would break out of the quoting. Windows filenames cannot
// contain one, which makes this an assertion rather than a filter — but the
// cost of being wrong is command execution, so assert it anyway.
func preflight(exe string) error {
	if strings.ContainsAny(exe, "\"\n\r") {
		return fmt.Errorf("installer: refusing to install from %q — the path must not contain quotes or newlines", exe)
	}

	return nil
}

func install(exe string) error {
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	logFile := filepath.Join(filepath.Dir(exe), "bits-wifi-login.log")

	tasks := []struct {
		name string
		xml  string
		desc string
	}{
		{mainTask, mainTaskXML(user, exe, logFile), "every 5 minutes and on login"},
		{eventTask, eventTaskXML(user, exe, logFile), "on network connect and on resume"},
	}

	for _, t := range tasks {
		path := filepath.Join(os.TempDir(), t.name+".xml")

		// schtasks /xml requires a Unicode file, not UTF-8.
		if err := os.WriteFile(path, utf16LE(t.xml), 0600); err != nil {
			return fmt.Errorf("installer: writing %s: %w", path, err)
		}

		err := run("schtasks", "/create", "/tn", t.name, "/xml", path, "/f")
		os.Remove(path)
		if err != nil {
			return err
		}

		fmt.Printf("✓ Scheduled task %s registered (%s).\n", t.name, t.desc)
	}

	return nil
}

func uninstall() error {
	for _, name := range []string{mainTask, eventTask} {
		if err := run("schtasks", "/delete", "/tn", name, "/f"); err != nil {
			fmt.Printf("⚠ Not found: scheduled task %s\n", name)
		} else {
			fmt.Printf("✓ Removed scheduled task %s\n", name)
		}
	}

	return nil
}

func summary() string {
	return "  Triggers:\n" +
		"    - Every WiFi connect (NetworkProfile Event ID 10000)\n" +
		"    - Every resume from sleep (Power-Troubleshooter Event ID 1)\n" +
		"    - Every 5 minutes, and on login\n\n" +
		"  Logs:\n" +
		"    Get-Content bits-wifi-login.log -Tail 50\n\n" +
		"  Repair:\n" +
		"    Re-run `bits-wifi-login install` if triggers or permissions break.\n"
}

// utf16LE encodes with a BOM, which is what schtasks expects from /xml.
func utf16LE(s string) []byte {
	encoded := utf16.Encode([]rune(s))

	out := make([]byte, 0, 2+len(encoded)*2)
	out = append(out, 0xFF, 0xFE)
	for _, r := range encoded {
		out = append(out, byte(r), byte(r>>8))
	}

	return out
}

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))

	return b.String()
}

// The binary logs to stdout and a scheduled task discards that, so route it
// through cmd.exe to keep a log file — Windows has no journald here.
func action(exe, logFile string) string {
	return fmt.Sprintf(`  <Actions Context="Author">
    <Exec>
      <Command>cmd.exe</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>`, esc(fmt.Sprintf(`/c ""%s" >> "%s" 2>&1"`, exe, logFile)))
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
  </Principals>`, esc(user))
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
`, startBoundary(), esc(user), principals(user), taskSettings, action(exe, logFile))
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

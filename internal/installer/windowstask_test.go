package installer

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// schtasks /xml rejects UTF-8 outright, so the encoding is load-bearing: wrong
// bytes here mean "the task file is invalid" with no hint as to why.
func TestUTF16LE(t *testing.T) {
	got := utf16LE("A<")

	want := []byte{0xFF, 0xFE, 'A', 0x00, '<', 0x00}
	if len(got) != len(want) {
		t.Fatalf("utf16LE(%q) = % x, want % x", "A<", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("utf16LE(%q) = % x, want % x", "A<", got, want)
		}
	}
}

func TestUTF16LEHandlesNonASCII(t *testing.T) {
	// A user name with an accent, and an emoji that needs a surrogate pair.
	for _, s := range []string{"José", "task 🎉"} {
		got := utf16LE(s)

		if got[0] != 0xFF || got[1] != 0xFE {
			t.Errorf("utf16LE(%q) lost the BOM", s)
		}
		if (len(got)-2)%2 != 0 {
			t.Errorf("utf16LE(%q) produced an odd number of payload bytes", s)
		}
	}
}

func taskDocs(logon string) map[string]string {
	return map[string]string{
		"main":  mainTaskXML(`DOMAIN\user`, `C:\Apps\bits-wifi-login.exe`, logon),
		"event": eventTaskXML(`DOMAIN\user`, `C:\Apps\bits-wifi-login.exe`, logon),
	}
}

// A malformed task document is rejected by schtasks with an unhelpful error, so
// prove both documents parse before they ever reach it.
func TestTaskXMLIsWellFormed(t *testing.T) {
	for name, doc := range taskDocs(logonS4U) {
		t.Run(name, func(t *testing.T) {
			dec := xml.NewDecoder(strings.NewReader(doc))
			// The declaration says UTF-16 but the Go string is UTF-8; the bytes are
			// transcoded only at write time, so pass the reader through untouched.
			dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }

			for {
				_, err := dec.Token()
				if err == io.EOF {
					return
				}
				if err != nil {
					t.Fatalf("%s task XML is not well-formed: %v", name, err)
				}
			}
		})
	}
}

// A domain\user containing & or < must not break the document.
func TestTaskXMLEscapesUser(t *testing.T) {
	doc := mainTaskXML(`DOM&AIN\a<b`, `C:\x.exe`, logonS4U)

	if strings.Contains(doc, `DOM&AIN`) {
		t.Error("raw & survived into the task XML — the document will not parse")
	}
	if !strings.Contains(doc, "DOM&amp;AIN") {
		t.Errorf("user name was not XML-escaped:\n%s", doc)
	}

	dec := xml.NewDecoder(strings.NewReader(doc))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("escaped document still not well-formed: %v", err)
		}
	}
}

// A shell in the action means a console window on screen every five minutes.
// Nothing may creep back between the scheduler and the binary.
func TestActionRunsTheBinaryDirectly(t *testing.T) {
	got := action(`C:\Program Files\bits-wifi-login.exe`)

	if !strings.Contains(got, `<Command>C:\Program Files\bits-wifi-login.exe</Command>`) {
		t.Errorf("action does not run the binary directly:\n%s", got)
	}
	if strings.Contains(strings.ToLower(got), "cmd.exe") ||
		strings.Contains(strings.ToLower(got), "powershell") {
		t.Errorf("a shell is back in the action — the console window returns with it:\n%s", got)
	}
	if strings.Contains(got, "<Arguments>") {
		t.Errorf("the binary takes no arguments from the scheduler:\n%s", got)
	}
	// <Command> is a path, not a command line: quoting it would look for a
	// binary whose name literally starts with a quote.
	if strings.Contains(command(t, got), `"`) {
		t.Errorf("the command path is quoted:\n%s", got)
	}
}

func command(t *testing.T, action string) string {
	t.Helper()

	_, rest, ok := strings.Cut(action, "<Command>")
	if !ok {
		t.Fatalf("no <Command> element in:\n%s", action)
	}
	cmd, _, ok := strings.Cut(rest, "</Command>")
	if !ok {
		t.Fatalf("unterminated <Command> element in:\n%s", action)
	}

	return cmd
}

// S4U is the whole reason the window is gone; a task written with the
// interactive logon type shows one again.
func TestTaskXMLCarriesTheLogonType(t *testing.T) {
	for _, logon := range []string{logonS4U, logonInteractive} {
		for name, doc := range taskDocs(logon) {
			if !strings.Contains(doc, "<LogonType>"+logon+"</LogonType>") {
				t.Errorf("%s task did not carry logon type %s:\n%s", name, logon, doc)
			}
		}
	}
}

// StartBoundary must be in the future; Task Scheduler silently refuses a
// TimeTrigger whose boundary has already passed.
func TestStartBoundaryFormat(t *testing.T) {
	got := startBoundary()

	if len(got) != len("2006-01-02T15:04:05") {
		t.Fatalf("startBoundary() = %q, want a bare local ISO-8601 timestamp", got)
	}
	if strings.HasSuffix(got, "Z") || strings.Contains(got, "+") {
		t.Errorf("startBoundary() = %q — Task Scheduler wants local time with no zone suffix", got)
	}
}

func TestTaskNamesAreStable(t *testing.T) {
	// uninstall() deletes by these exact names, and so does the documentation.
	if mainTask != "BITS-WiFi-Login" || eventTask != "BITS-WiFi-Login-OnConnect" {
		t.Errorf("task names changed (%q, %q) — uninstall and the README refer to the old ones",
			mainTask, eventTask)
	}
}

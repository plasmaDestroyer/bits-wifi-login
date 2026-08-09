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

// A malformed task document is rejected by schtasks with an unhelpful error, so
// prove both documents parse before they ever reach it.
func TestTaskXMLIsWellFormed(t *testing.T) {
	docs := map[string]string{
		"main":  mainTaskXML(`DOMAIN\user`, `C:\Apps\bits-wifi-login.exe`, `C:\Apps\bits-wifi-login.log`),
		"event": eventTaskXML(`DOMAIN\user`, `C:\Apps\bits-wifi-login.exe`, `C:\Apps\bits-wifi-login.log`),
	}

	for name, doc := range docs {
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
	doc := mainTaskXML(`DOM&AIN\a<b`, `C:\x.exe`, `C:\x.log`)

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

// The `cmd /c "..."` idiom: cmd strips the outermost quote pair, leaving each
// path quoted individually. Collapsing the doubled quotes breaks any path with
// a space in it — which is most Windows paths.
func TestTaskArgumentsQuoting(t *testing.T) {
	got := taskArguments(`C:\Program Files\bits-wifi-login.exe`, `C:\Program Files\bits.log`)

	want := `/c ""C:\Program Files\bits-wifi-login.exe" >> "C:\Program Files\bits.log" 2>&1"`
	if got != want {
		t.Errorf("taskArguments() =\n  %s\nwant\n  %s", got, want)
	}
	if !strings.HasPrefix(got, `/c ""`) {
		t.Error("lost the doubled opening quote — a path with a space will not run")
	}
	if !strings.HasSuffix(got, `2>&1"`) {
		t.Error("lost the trailing quote that closes the /c string")
	}
}

// The action element carries the arguments through XML escaping; >> and & must
// survive as entities, not raw.
func TestActionEscapesRedirection(t *testing.T) {
	got := action(`C:\x.exe`, `C:\x.log`)

	if strings.Contains(got, `2>&1<`) || strings.Contains(got, `>> "`) {
		t.Errorf("redirection was not XML-escaped:\n%s", got)
	}
	if !strings.Contains(got, "&gt;&gt;") || !strings.Contains(got, "&amp;") {
		t.Errorf("expected escaped >> and & in:\n%s", got)
	}
	if !strings.Contains(got, "<Command>cmd.exe</Command>") {
		t.Errorf("action does not invoke cmd.exe:\n%s", got)
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

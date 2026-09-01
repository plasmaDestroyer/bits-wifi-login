package installer

import (
	"strings"
	"testing"
)

// type feeds a whole string through key() the way the read loop does, and
// returns what was stored and what the terminal would have shown.
func typeIn(s string) (typed string, screen string) {
	var buf []byte
	var out strings.Builder

	for i := 0; i < len(s); i++ {
		next, echo, done := key(buf, s[i])
		buf = next
		out.WriteString(echo)

		if done {
			break
		}
	}

	return string(buf), out.String()
}

func TestKey(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantTyped  string
		wantScreen string
	}{
		{
			name:       "one star per character",
			input:      "hunter2",
			wantTyped:  "hunter2",
			wantScreen: "*******",
		},
		{
			// A rune is up to four bytes and key() sees one at a time, so a naive
			// implementation draws four stars for one character.
			name:       "a multi-byte character is one star",
			input:      "aé☃",
			wantTyped:  "aé☃",
			wantScreen: "***",
		},
		{
			name:       "backspace erases",
			input:      "abc\x7f",
			wantTyped:  "ab",
			wantScreen: "***\b \b",
		},
		{
			// Erasing a byte at a time would leave half a rune behind and corrupt
			// the password without showing anything on screen.
			name:       "backspace erases a whole rune",
			input:      "aé\x7f",
			wantTyped:  "a",
			wantScreen: "**\b \b",
		},
		{
			name:       "backspace on an empty password does nothing",
			input:      "\x7f\x7f",
			wantTyped:  "",
			wantScreen: "",
		},
		{
			name:       "ctrl-u clears the line",
			input:      "abc\x15",
			wantTyped:  "",
			wantScreen: "***" + "\b \b\b \b\b \b",
		},
		{
			// Arrow keys and the like arrive as control bytes. Storing them would
			// put invisible junk in the password.
			name:       "other control bytes are ignored",
			input:      "a\x01\x1bb",
			wantTyped:  "ab",
			wantScreen: "**",
		},
		{
			name:       "enter ends the line",
			input:      "ab\rcd",
			wantTyped:  "ab",
			wantScreen: "**\r\n",
		},
		{
			name:       "spaces are real characters",
			input:      "a b",
			wantTyped:  "a b",
			wantScreen: "***",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			typed, screen := typeIn(c.input)

			if typed != c.wantTyped {
				t.Errorf("typed = %q, want %q", typed, c.wantTyped)
			}
			if screen != c.wantScreen {
				t.Errorf("screen = %q, want %q", screen, c.wantScreen)
			}
		})
	}
}

// Ctrl-C must not be mistaken for a submitted password.
func TestKeyCtrlCAbandonsTheInput(t *testing.T) {
	out, _, done := key([]byte("secret"), keyCtrlC)

	if !done {
		t.Error("key() did not finish on Ctrl-C")
	}
	if out != nil {
		t.Errorf("key() = %q on Ctrl-C, want nil so it cannot be read as a password", out)
	}
}

func TestStars(t *testing.T) {
	for input, want := range map[string]int{"": 0, "abc": 3, "aé☃": 3, "☃☃": 2} {
		if got := stars([]byte(input)); got != want {
			t.Errorf("stars(%q) = %d, want %d", input, got, want)
		}
	}
}

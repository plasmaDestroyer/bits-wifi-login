package installer

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/term"
)

// Windows and macOS echo a star per character, since a dead-silent prompt looks
// frozen. Linux keeps the silent read: that is what sudo does.

const (
	keyCtrlC     = 3
	keyBackspace = 8
	keyCtrlU     = 21
	keyDelete    = 127
)

// key folds one byte into the password and says what to echo. One star per
// rune, not per byte: continuation bytes (0b10xxxxxx) draw nothing.
func key(typed []byte, b byte) (out []byte, echo string, done bool) {
	switch b {
	case '\r', '\n':
		return typed, "\r\n", true

	case keyBackspace, keyDelete:
		if len(typed) == 0 {
			return typed, "", false
		}

		// Drop a whole rune, not a byte.
		cut := len(typed) - 1
		for cut > 0 && typed[cut]&0xC0 == 0x80 {
			cut--
		}

		return typed[:cut], "\b \b", false

	case keyCtrlU:
		echo = ""
		for range stars(typed) {
			echo += "\b \b"
		}

		return typed[:0], echo, false

	case keyCtrlC:
		return nil, "\r\n", true
	}

	if b < 0x20 {
		return typed, "", false
	}

	if b&0xC0 == 0x80 {
		return append(typed, b), "", false
	}

	return append(typed, b), "*", false
}

func stars(typed []byte) int {
	n := 0
	for _, b := range typed {
		if b&0xC0 != 0x80 {
			n++
		}
	}

	return n
}

// readMasked reads a password with stars on Windows/macOS terminals, silently otherwise.
func readMasked(fd int) ([]byte, error) {
	if runtime.GOOS == "linux" {
		return term.ReadPassword(fd)
	}

	if !term.IsTerminal(fd) {
		return term.ReadPassword(fd)
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return term.ReadPassword(fd)
	}
	// Restore the terminal on every way out, panic included.
	defer term.Restore(fd, state)

	var typed []byte
	buf := make([]byte, 1)

	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}

		next, echo, done := key(typed, buf[0])
		typed = next
		fmt.Print(echo)

		if done {
			if typed == nil { // Ctrl-C
				term.Restore(fd, state)
				os.Exit(1)
			}

			return typed, nil
		}
	}
}

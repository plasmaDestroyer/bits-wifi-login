package installer

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/term"
)

// Reading a password with x/term echoes nothing at all — not even the cursor
// moving — which is indistinguishable from a frozen terminal to anyone who has
// not met it before. So on Windows and macOS do what a password field on a web
// page does and echo a star per character. It reveals only the length, which the
// person typing it already knows.
//
// Linux keeps the silent read. A silent password prompt is the convention every
// Unix tool follows, sudo included, and anyone installing a CLI there has met it
// — so stars would be the surprising behaviour, not the reassuring one.

const (
	keyCtrlC     = 3
	keyBackspace = 8
	keyCtrlU     = 21
	keyDelete    = 127
)

// key folds one byte into the password being typed and says what to echo for it.
// Pure, so every editing case is testable without a terminal.
//
// A rune of UTF-8 is one to four bytes and this is fed one byte at a time, so
// echo a star only for a byte that begins a rune — continuation bytes are
// 0b10xxxxxx. Otherwise a non-ASCII password draws four stars for one character.
func key(typed []byte, b byte) (out []byte, echo string, done bool) {
	switch b {
	case '\r', '\n':
		return typed, "\r\n", true

	case keyBackspace, keyDelete:
		if len(typed) == 0 {
			return typed, "", false
		}

		// Drop a whole rune, not a byte, or a backspace over a non-ASCII
		// character leaves a broken fragment behind.
		cut := len(typed) - 1
		for cut > 0 && typed[cut]&0xC0 == 0x80 {
			cut--
		}

		return typed[:cut], "\b \b", false

	case keyCtrlU: // clear the line, as a shell does
		echo = ""
		for range stars(typed) {
			echo += "\b \b"
		}

		return typed[:0], echo, false

	case keyCtrlC:
		return nil, "\r\n", true
	}

	if b < 0x20 { // any other control character: ignore rather than store
		return typed, "", false
	}

	if b&0xC0 == 0x80 { // UTF-8 continuation: store it, but it is not a new char
		return append(typed, b), "", false
	}

	return append(typed, b), "*", false
}

// stars is how many characters have been typed, counting a multi-byte rune once.
func stars(typed []byte) int {
	n := 0
	for _, b := range typed {
		if b&0xC0 != 0x80 {
			n++
		}
	}

	return n
}

// readMasked reads a password from a terminal, echoing a star per character.
// Falls back to x/term's silent read when stdin is not a terminal, which is what
// happens under a pipe and in tests.
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
	// Raw mode disables line editing and Ctrl-C for the whole terminal, so it has
	// to be restored on every path out of here, panic included.
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

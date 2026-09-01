package installer

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

// A step that prints nothing while it waits reads as a hang, and the install has
// one that genuinely takes seconds: asking NetworkManager to probe, which under a
// VPN blocks until it gives up. Keep something moving so the difference between
// "working" and "wedged" is visible.

const frameRate = 100 * time.Millisecond

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type spinner struct {
	stop     chan struct{}
	finished chan struct{}
}

// spin starts an animated line, or prints a plain one when stdout is not a
// terminal — a redirected install log should not collect thousands of carriage
// returns. A nil spinner is valid and means exactly that case.
func spin(msg string) *spinner {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}

	s := &spinner{stop: make(chan struct{}), finished: make(chan struct{})}

	go func() {
		defer close(s.finished)

		for i := 0; ; i++ {
			select {
			case <-s.stop:
				return
			case <-time.After(frameRate):
			}

			fmt.Printf("\r%s %s", frames[i%len(frames)], msg)
		}
	}()

	return s
}

// done stops the animation and leaves final in its place. Always called, so the
// cursor never ends up parked on a half-drawn frame.
func (s *spinner) done(final string) {
	if s == nil {
		fmt.Println(final)
		return
	}

	close(s.stop)
	<-s.finished

	// \r to the margin, then erase to end of line: the finished text can be
	// shorter than the frame it replaces, and leftovers would trail after it.
	fmt.Printf("\r\033[K%s\n", final)
}

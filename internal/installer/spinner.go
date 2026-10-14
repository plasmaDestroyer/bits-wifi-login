package installer

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

const frameRate = 100 * time.Millisecond

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type spinner struct {
	stop     chan struct{}
	finished chan struct{}
}

// spin starts an animated line, or returns nil when stdout is not a terminal.
// A nil spinner is valid.
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

func (s *spinner) done(final string) {
	if s == nil {
		fmt.Println(final)
		return
	}

	close(s.stop)
	<-s.finished

	// Erase the line: final can be shorter than the frame.
	fmt.Printf("\r\033[K%s\n", final)
}

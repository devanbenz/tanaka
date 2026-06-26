// Package ui provides small terminal feedback helpers.
package ui

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"
	"time"
)

// kaomojiSet is a curated set of animated kaomoji; each is a list of frames.
var kaomojiSet = [][]string{
	{"┌(･o･)┘", "└(･o･)┐", "┌(･o･)┐", "└(･o･)┘"},
	{"(・_・)", "(・_・ )", "( ・_・)", "(・_・)"},
	{"┐(･ω･)┌", "┌(･ω･)┐"},
	{"(>_<)", "(>ω<)", "(>﹏<)"},
	{"(๑•̀ㅂ•́)و", "(๑•̀ㅂ•́)൬"},
}

// nextKaomoji returns an index in [0,n) different from cur (when n>1).
func nextKaomoji(cur, n int, r *rand.Rand) int {
	if n <= 1 {
		return 0
	}
	for {
		k := r.Intn(n)
		if k != cur {
			return k
		}
	}
}

const (
	doneFace = `\(^_^)/`
	failFace = `(>_<)`
	tick     = 150 * time.Millisecond
)

// Spinner shows a phase message with an animated kaomoji on a TTY, and falls
// back to plain one-line phase messages when the writer is not a terminal.
type Spinner struct {
	w     io.Writer
	tty   bool
	msg   string
	start time.Time
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

// NewSpinner returns a Spinner writing to w. Animation is enabled only when w is
// a terminal.
func NewSpinner(w io.Writer, msg string) *Spinner {
	return &Spinner{w: w, tty: isTTY(w), msg: msg}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// frameLine renders one animation frame: a carriage return so it overwrites in
// place, the kaomoji frame, the message, and elapsed seconds.
func frameLine(frame, msg string, elapsed time.Duration) string {
	return fmt.Sprintf("\r%s  %s  (%ds) ", frame, msg, int(elapsed.Seconds()))
}

// Start begins the phase. On a TTY it animates until Stop/Fail; otherwise it
// prints a single plain phase line.
func (s *Spinner) Start() {
	s.start = time.Now()
	if !s.tty {
		fmt.Fprintf(s.w, "%s ...\n", s.msg)
		return
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		cur := r.Intn(len(kaomojiSet))
		frame := 0
		lastRotate := time.Now()
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				if time.Since(lastRotate) > 3*time.Second {
					cur = nextKaomoji(cur, len(kaomojiSet), r)
					frame = 0
					lastRotate = time.Now()
				}
				frames := kaomojiSet[cur]
				fmt.Fprint(s.w, frameLine(frames[frame%len(frames)], s.msg, time.Since(s.start)))
				frame++
			}
		}
	}()
}

// Stop ends the phase with a success kaomoji and a final message.
func (s *Spinner) Stop(final string) { s.finish(doneFace, final) }

// Fail ends the phase with a failure kaomoji and a final message.
func (s *Spinner) Fail(final string) { s.finish(failFace, final) }

func (s *Spinner) finish(face, final string) {
	if !s.tty {
		fmt.Fprintf(s.w, "%s %s\n", face, final)
		return
	}
	s.once.Do(func() {
		close(s.stop)
		<-s.done
	})
	// Clear the animated line, then print the final line.
	fmt.Fprintf(s.w, "\r\033[K%s  %s  (%ds)\n", face, final, int(time.Since(s.start).Seconds()))
}

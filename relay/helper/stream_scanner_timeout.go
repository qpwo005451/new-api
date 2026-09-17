package helper

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"time"
)

// ErrStreamIdleTimeout indicates that no complete line arrived before the
// configured streaming timeout. The scanner closes the upstream body to
// unblock a pending Read.
var ErrStreamIdleTimeout = errors.New("stream idle timeout")

// IdleTimeoutScanner provides line-oriented scanning with an idle timeout.
// Each Scan call starts a fresh timer for the time spent waiting for the next
// line. When the timer fires, the upstream reader is closed so a blocked Scan
// can return.
type IdleTimeoutScanner struct {
	scanner *bufio.Scanner
	reader  io.ReadCloser
	timeout time.Duration

	mu         sync.Mutex
	timer      *time.Timer
	timerDone  chan struct{}
	generation uint64
	timedOut   bool
	stopped    bool
}

// NewStreamScannerWithIdleTimeout creates a line scanner that aborts a read
// after timeout with no complete line. A non-positive timeout disables the
// idle watchdog.
func NewStreamScannerWithIdleTimeout(reader io.ReadCloser, timeout time.Duration) *IdleTimeoutScanner {
	s := &IdleTimeoutScanner{
		scanner: NewStreamScanner(reader),
		reader:  reader,
		timeout: timeout,
	}
	return s
}

// Scan advances to the next line or returns false when the reader ends or the
// idle timeout fires.
func (s *IdleTimeoutScanner) Scan() bool {
	if s == nil || s.scanner == nil {
		return false
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return false
	}
	s.armLocked()
	s.mu.Unlock()

	ok := s.scanner.Scan()

	s.mu.Lock()
	s.generation++
	timedOut := s.timedOut
	timerStopped := true
	if s.timer != nil {
		timerStopped = s.timer.Stop()
		s.timer = nil
	}
	timerDone := s.timerDone
	s.timerDone = nil
	s.mu.Unlock()
	if !timerStopped && timerDone != nil {
		<-timerDone
	}

	if timedOut {
		return false
	}
	return ok
}

// Text returns the most recent line.
func (s *IdleTimeoutScanner) Text() string {
	if s == nil || s.scanner == nil {
		return ""
	}
	return s.scanner.Text()
}

// Err returns the idle timeout when the watchdog fired; otherwise it returns
// the underlying scanner error.
func (s *IdleTimeoutScanner) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	timedOut := s.timedOut
	s.mu.Unlock()
	if timedOut {
		return ErrStreamIdleTimeout
	}
	if s.scanner == nil {
		return nil
	}
	return s.scanner.Err()
}

// Stop stops the idle watchdog without closing the underlying reader.
func (s *IdleTimeoutScanner) Stop() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.generation++
	timerStopped := true
	timer := s.timer
	s.timer = nil
	timerDone := s.timerDone
	s.timerDone = nil
	s.mu.Unlock()

	if timer != nil {
		timerStopped = timer.Stop()
	}
	if !timerStopped && timerDone != nil {
		<-timerDone
	}
}

func (s *IdleTimeoutScanner) armLocked() {
	if s.stopped || s.timeout <= 0 {
		return
	}
	s.generation++
	generation := s.generation
	timerDone := make(chan struct{})
	s.timerDone = timerDone
	s.timer = time.AfterFunc(s.timeout, func() {
		defer close(timerDone)
		s.timeoutExpired(generation)
	})
}

func (s *IdleTimeoutScanner) timeoutExpired(generation uint64) {
	s.mu.Lock()
	if s.stopped || s.timedOut || generation != s.generation {
		s.mu.Unlock()
		return
	}
	s.timedOut = true
	reader := s.reader
	s.mu.Unlock()

	if reader != nil {
		_ = reader.Close()
	}
}

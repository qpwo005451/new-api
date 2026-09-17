package helper

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdleTimeoutScannerResetsOnData(t *testing.T) {
	pr, pw := io.Pipe()
	scanner := NewStreamScannerWithIdleTimeout(pr, 50*time.Millisecond)
	t.Cleanup(func() {
		scanner.Stop()
		_ = pw.Close()
	})

	result := make(chan bool, 1)
	go func() {
		result <- scanner.Scan()
	}()

	_, err := pw.Write([]byte("first\n"))
	require.NoError(t, err)
	select {
	case ok := <-result:
		require.True(t, ok)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first line")
	}

	time.Sleep(30 * time.Millisecond)
	result = make(chan bool, 1)
	go func() {
		result <- scanner.Scan()
	}()
	_, err = pw.Write([]byte("second\n"))
	require.NoError(t, err)
	select {
	case ok := <-result:
		require.True(t, ok)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second line")
	}
	require.NoError(t, scanner.Err())
	scanner.Stop()
	assert.False(t, scanner.Scan())
}

func TestIdleTimeoutScannerDoesNotTimeoutAfterLineArrives(t *testing.T) {
	pr, pw := io.Pipe()
	scanner := NewStreamScannerWithIdleTimeout(pr, 50*time.Millisecond)
	t.Cleanup(func() {
		scanner.Stop()
		_ = pw.Close()
		_ = pr.Close()
	})

	result := make(chan bool, 1)
	go func() {
		result <- scanner.Scan()
	}()

	time.Sleep(45 * time.Millisecond)
	_, err := pw.Write([]byte("line\n"))
	require.NoError(t, err)
	select {
	case ok := <-result:
		require.True(t, ok)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for line")
	}

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, scanner.Err())
}

func TestIdleTimeoutScannerTimesOutBlockedScan(t *testing.T) {
	pr, pw := io.Pipe()
	scanner := NewStreamScannerWithIdleTimeout(pr, 20*time.Millisecond)
	t.Cleanup(func() {
		scanner.Stop()
		_ = pw.Close()
	})

	result := make(chan bool, 1)
	go func() {
		result <- scanner.Scan()
	}()

	select {
	case ok := <-result:
		assert.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("scanner did not unblock after idle timeout")
	}
	require.ErrorIs(t, scanner.Err(), ErrStreamIdleTimeout)
}

func TestIdleTimeoutScannerDisabledTimeout(t *testing.T) {
	scanner := NewStreamScannerWithIdleTimeout(io.NopCloser(strings.NewReader("line\n")), 0)
	t.Cleanup(scanner.Stop)

	require.True(t, scanner.Scan())
	assert.Equal(t, "line", scanner.Text())
	require.NoError(t, scanner.Err())
}

func TestIdleTimeoutScannerStopStopsTimer(t *testing.T) {
	pr, _ := io.Pipe()
	scanner := NewStreamScannerWithIdleTimeout(pr, time.Hour)

	scanner.Stop()
	scanner.Stop()

	assert.False(t, scanner.Scan())
	assert.NotErrorIs(t, scanner.Err(), ErrStreamIdleTimeout)
}

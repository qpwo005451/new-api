package helper

import (
	"strings"
	"unicode/utf8"
)

// Degenerate-loop detection tuning.
//
// Upstream channels can occasionally fall into a generation loop, emitting the
// same short phrase ("let me ", ...) until the output cap is reached. Such a
// stream still ends with finish_reason=length, so truncation checks alone
// cannot see it. The detector below watches the decoded body text and stops the
// stream once a short unit repeats enough times to dominate the recent output.
//
// Tuning direction: raising maxPeriodBytes, minRepeats or minTotalBytes makes
// the detector more conservative (fewer false positives, later stop); lowering
// them makes it stop sooner but risks flagging legitimate repetition. The
// suffix ratio is the main guard against flagging a normal response that merely
// ends with a long repeated code block.
const (
	// loopDetectorWindowBytes is the rolling window of recently emitted body
	// text that the periodicity check runs against.
	loopDetectorWindowBytes = 4 << 10
	// loopDetectorMaxPeriodBytes is the largest repeated unit (in bytes) the
	// detector considers. It must comfortably cover short phrases while still
	// allowing a repeated sentence.
	loopDetectorMaxPeriodBytes = 256
	// loopDetectorMinRepeats is the minimum number of complete back-to-back
	// repetitions required to call the tail degenerate.
	loopDetectorMinRepeats = 4
	// loopDetectorMinTotalBytes is the minimum length of the repeated run.
	// It keeps very short accidental repeats from tripping the detector.
	loopDetectorMinTotalBytes = 600
	// loopDetectorMinSuffixPercent is the minimum share of the rolling window
	// that the repeated run must cover. Requiring the run to dominate the
	// window is what distinguishes a real loop from a long repeated block at
	// the end of an otherwise varied response.
	loopDetectorMinSuffixPercent = 80
	// loopDetectorSnippetBytes bounds the excerpt kept for logging.
	loopDetectorSnippetBytes = 80
)

// loopDetectorModelPrefixes is the model whitelist for the DeepSeek-V4.1-flash
// and DeepSeek-V4-flash families. Detection is opt-in so the extra parsing and
// scanning never run for providers that have not shown this failure mode.
// Matching is case-insensitive and tolerates an organization prefix such as
// "deepseek/deepseek-v4.1-flash". Keep the more specific v4.1 entry before the
// broader v4 entry because LoopDetectorEnabled checks prefixes in order.
var loopDetectorModelPrefixes = []string{
	"deepseek-v4.1-flash",
	"deepseek-v4-flash",
}

// LoopDetector accumulates the decoded body text (content and reasoning
// content) of one streamed response and reports when the tail degenerates into
// a short unit repeated many times. It is transport agnostic: the caller feeds
// text and stops the upstream stream when Append reports a loop.
type LoopDetector struct {
	window    []byte
	triggered bool
	period    int
	repeats   int
	snippet   string
}

// NewLoopDetector returns an empty detector.
func NewLoopDetector() *LoopDetector {
	return &LoopDetector{}
}

// LoopDetectorEnabled reports whether the model is on the detection whitelist.
// Matching ignores case and an optional organization prefix.
func LoopDetectorEnabled(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return false
	}
	if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
		name = name[idx+1:]
	}
	for _, prefix := range loopDetectorModelPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// Append adds decoded body text to the rolling window and reports whether the
// tail is now a degenerate repetition. It reports true at most once; later
// calls are ignored so a single response stops on the first detection.
func (d *LoopDetector) Append(text string) bool {
	if d == nil || d.triggered || text == "" {
		return false
	}

	d.window = append(d.window, text...)
	if len(d.window) > loopDetectorWindowBytes {
		drop := len(d.window) - loopDetectorWindowBytes
		copy(d.window, d.window[drop:])
		d.window = d.window[:loopDetectorWindowBytes]
	}

	period, repeats, ok := detectLoop(d.window)
	if !ok {
		return false
	}

	d.triggered = true
	d.period = period
	d.repeats = repeats
	d.snippet = truncateRuneSafe(d.window[len(d.window)-period:], loopDetectorSnippetBytes)
	return true
}

// Triggered reports whether a loop has already been detected.
func (d *LoopDetector) Triggered() bool {
	return d != nil && d.triggered
}

// MatchPeriod is the byte length of the repeated unit of the detection.
func (d *LoopDetector) MatchPeriod() int {
	if d == nil {
		return 0
	}
	return d.period
}

// MatchRepeats is the number of complete repetitions of the detection.
func (d *LoopDetector) MatchRepeats() int {
	if d == nil {
		return 0
	}
	return d.repeats
}

// MatchSnippet is a rune-safe, length-bounded excerpt of the repeated unit.
func (d *LoopDetector) MatchSnippet() string {
	if d == nil {
		return ""
	}
	return d.snippet
}

// detectLoop returns the smallest period whose run of complete repetitions
// dominates the tail of window.
func detectLoop(window []byte) (period int, repeats int, ok bool) {
	if len(window) < loopDetectorMinTotalBytes {
		return 0, 0, false
	}

	// A run of minRepeats complete periods cannot be longer than the window,
	// which bounds how large a candidate period can be.
	maxPeriod := min(loopDetectorMaxPeriodBytes, len(window)/loopDetectorMinRepeats)
	for p := 1; p <= maxPeriod; p++ {
		// The period cut must land on a rune boundary so a multi-byte
		// character is never split into a bogus repeated unit.
		if !utf8.RuneStart(window[len(window)-p]) {
			continue
		}

		// Count how many trailing bytes match the byte p positions earlier.
		// The last block is always one repeat; each full matching block adds
		// another.
		match := 0
		for i := len(window) - 1; i >= p; i-- {
			if window[i] != window[i-p] {
				break
			}
			match++
		}
		repeats = match/p + 1
		if repeats < loopDetectorMinRepeats {
			continue
		}

		total := repeats * p
		if total < loopDetectorMinTotalBytes {
			continue
		}
		if total*100 < loopDetectorMinSuffixPercent*len(window) {
			continue
		}
		return p, repeats, true
	}
	return 0, 0, false
}

// truncateRuneSafe returns at most maxBytes leading bytes of b without cutting a
// multi-byte rune.
func truncateRuneSafe(b []byte, maxBytes int) string {
	if len(b) <= maxBytes {
		return string(b)
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(b[cut]) {
		cut--
	}
	return string(b[:cut])
}

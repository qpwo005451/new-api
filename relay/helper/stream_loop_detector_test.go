package helper_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoopDetectorTriggersOnRepeatedPhrase(t *testing.T) {
	detector := helper.NewLoopDetector()

	require.True(t, detector.Append(strings.Repeat("let me ", 100)))

	assert.True(t, detector.Triggered())
	assert.Equal(t, 7, detector.MatchPeriod())
	assert.Equal(t, 100, detector.MatchRepeats())
	assert.Equal(t, "let me ", detector.MatchSnippet())

	// A single response stops on the first detection.
	assert.False(t, detector.Append(strings.Repeat("let me ", 100)))
}

func TestLoopDetectorTriggersAcrossChunkedDeltas(t *testing.T) {
	detector := helper.NewLoopDetector()

	triggered := false
	for range 200 {
		if detector.Append("let me ") {
			triggered = true
			break
		}
	}

	require.True(t, triggered)
	assert.Equal(t, 7, detector.MatchPeriod())
	assert.GreaterOrEqual(t, detector.MatchRepeats(), 4)
}

func TestLoopDetectorIgnoresShortRepeats(t *testing.T) {
	detector := helper.NewLoopDetector()

	// 80 repeats are only 560 bytes, below the 600 byte minimum run.
	assert.False(t, detector.Append(strings.Repeat("let me ", 80)))
	assert.False(t, detector.Triggered())
}

func TestLoopDetectorIgnoresRepeatedTailInVariedWindow(t *testing.T) {
	detector := helper.NewLoopDetector()

	var varied strings.Builder
	for i := range 600 {
		fmt.Fprintf(&varied, "%d;", i*37)
	}
	require.Greater(t, varied.Len(), 2000)
	require.False(t, detector.Append(varied.String()))

	// A long repeated block at the end of an otherwise varied response does not
	// dominate the window, so it must not be mistaken for a loop.
	assert.False(t, detector.Append(strings.Repeat("=", 700)))
	assert.False(t, detector.Triggered())
}

func TestLoopDetectorDetectsMultiByteUTF8Period(t *testing.T) {
	const unit = "你好吗？" // 4 runes, 12 bytes

	detector := helper.NewLoopDetector()
	require.True(t, detector.Append(strings.Repeat(unit, 60)))

	assert.Equal(t, len(unit), detector.MatchPeriod())
	assert.Equal(t, 60, detector.MatchRepeats())
	assert.Equal(t, unit, detector.MatchSnippet())
	assert.True(t, utf8.ValidString(detector.MatchSnippet()))
}

func TestLoopDetectorDetectsRepeatedMultiByteRune(t *testing.T) {
	detector := helper.NewLoopDetector()
	require.True(t, detector.Append(strings.Repeat("é", 400)))

	assert.Equal(t, 2, detector.MatchPeriod())
	assert.Equal(t, 400, detector.MatchRepeats())
	assert.Equal(t, "é", detector.MatchSnippet())
}

func TestLoopDetectorSnippetNeverSplitsRune(t *testing.T) {
	// 30 distinct 3-byte runes form an aperiodic 90-byte unit, so the reported
	// period is the whole unit and the 80-byte snippet must be trimmed down to a
	// rune boundary.
	var unitBuilder strings.Builder
	for i := range 30 {
		unitBuilder.WriteRune(rune(0x4E00 + i*7))
	}
	unit := unitBuilder.String()
	require.Equal(t, 90, len(unit))

	detector := helper.NewLoopDetector()
	require.True(t, detector.Append(strings.Repeat(unit, 12)))
	assert.Equal(t, len(unit), detector.MatchPeriod())

	snippet := detector.MatchSnippet()
	assert.LessOrEqual(t, len(snippet), 80)
	assert.True(t, utf8.ValidString(snippet))
	assert.Equal(t, 26, utf8.RuneCountInString(snippet))
}

func TestLoopDetectorEnabledWhitelist(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4.1-flash", true},
		{"DeepSeek-V4.1-Flash", true},
		{"deepseek-v4.1-flash-none", true},
		{"deepseek-v4.1-flash-max", true},
		{"deepseek-ai/DeepSeek-V4.1-Flash", true},
		{"deepseek/deepseek-v4.1-flash-max", true},
		{"deepseek-v4-flash", true},
		{"deepseek-v4-flash-max", true},
		{"deepseek-v4-flash-none", true},
		{"deepseek-v4.1-pro", false},
		{"gpt-4o", false},
		{"deepseek-v4.1", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			assert.Equal(t, tc.want, helper.LoopDetectorEnabled(tc.model))
		})
	}
}

func TestStreamEndReasonUpstreamLoopIsNotNormal(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonUpstreamLoop, errors.New("upstream loop detected"))

	assert.False(t, status.IsNormalEnd())
	assert.Contains(t, status.Summary(), "upstream_loop")
}

type trackingReadCloser struct {
	reader io.Reader
	closed atomic.Bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

func newLoopStreamTestContext(t *testing.T, model string, body io.Reader) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo, *trackingReadCloser) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	upstream := &trackingReadCloser{reader: body}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       upstream,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
		},
	}
	return c, recorder, resp, info, upstream
}

func setLoopStreamTestTimeout(t *testing.T) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})
}

func repetitiveChatStream(model, content string, repeats int) string {
	var b strings.Builder
	for range repeats {
		fmt.Fprintf(&b,
			"data: {\"id\":\"chatcmpl-loop\",\"object\":\"chat.completion.chunk\",\"created\":1710000000,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n",
			model, content)
	}
	return b.String()
}

func TestOaiStreamHandlerStopsDegenerateLoop(t *testing.T) {
	setLoopStreamTestTimeout(t)

	const model = "deepseek-v4.1-flash"
	body := repetitiveChatStream(model, "let me ", 200)
	c, recorder, resp, info, upstream := newLoopStreamTestContext(t, model, strings.NewReader(body))

	usage, apiErr := openai.OaiStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonUpstreamLoop, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.ResponseFailed())
	assert.True(t, upstream.closed.Load(), "the upstream body must be closed to stop the loop")

	downstream := recorder.Body.String()
	assert.Contains(t, downstream, "upstream_loop_detected")
	assert.Contains(t, downstream, "upstream_stream_error")
	assert.NotContains(t, downstream, "data: [DONE]")
}

func TestOaiStreamHandlerDoesNotDetectLoopForOtherModels(t *testing.T) {
	setLoopStreamTestTimeout(t)

	// gpt-4o-mini is deliberately outside the detection whitelist.
	const model = "gpt-4o-mini"
	body := repetitiveChatStream(model, "let me ", 200) + "data: [DONE]\n\n"
	c, recorder, resp, info, _ := newLoopStreamTestContext(t, model, strings.NewReader(body))

	usage, apiErr := openai.OaiStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)

	downstream := recorder.Body.String()
	assert.NotContains(t, downstream, "upstream_loop_detected")
	assert.Contains(t, downstream, "data: [DONE]")
}

func TestOaiStreamHandlerKeepsNormalFinishForWhitelistedModel(t *testing.T) {
	setLoopStreamTestTimeout(t)

	const model = "deepseek-v4.1-flash"
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info, _ := newLoopStreamTestContext(t, model, strings.NewReader(body))

	usage, apiErr := openai.OaiStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)

	downstream := recorder.Body.String()
	assert.Contains(t, downstream, `"content":"OK"`)
	assert.Contains(t, downstream, "data: [DONE]")
	assert.NotContains(t, downstream, `"error"`)
}

func reasoningChatStream(model, content string, repeats int) string {
	var b strings.Builder
	for range repeats {
		fmt.Fprintf(&b,
			"data: {\"id\":\"chatcmpl-loop\",\"object\":\"chat.completion.chunk\",\"created\":1710000000,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":%q},\"finish_reason\":null}]}\n\n",
			model, content)
	}
	return b.String()
}

func toolCallChatStream(model, arguments string, repeats int) string {
	var b strings.Builder
	for range repeats {
		fmt.Fprintf(&b,
			"data: {\"id\":\"chatcmpl-tools\",\"object\":\"chat.completion.chunk\",\"created\":1710000000,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":%q}}]},\"finish_reason\":null}]}\n\n",
			model, arguments)
	}
	fmt.Fprintf(&b,
		"data: {\"id\":\"chatcmpl-tools\",\"object\":\"chat.completion.chunk\",\"created\":1710000000,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n",
		model)
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func TestOaiStreamHandlerDetectsLoopInReasoningContent(t *testing.T) {
	setLoopStreamTestTimeout(t)

	const model = "deepseek-v4.1-flash"
	body := reasoningChatStream(model, "let me ", 200)
	c, recorder, resp, info, _ := newLoopStreamTestContext(t, model, strings.NewReader(body))

	usage, apiErr := openai.OaiStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonUpstreamLoop, info.StreamStatus.EndReason)
	assert.Contains(t, recorder.Body.String(), "upstream_loop_detected")
}

func TestOaiStreamHandlerIgnoresToolCallArguments(t *testing.T) {
	setLoopStreamTestTimeout(t)

	const model = "deepseek-v4.1-flash"
	body := toolCallChatStream(model, "let me ", 200)
	c, recorder, resp, info, _ := newLoopStreamTestContext(t, model, strings.NewReader(body))

	usage, apiErr := openai.OaiStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.NotContains(t, recorder.Body.String(), "upstream_loop_detected")
}

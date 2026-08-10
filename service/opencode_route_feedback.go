package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const (
	openCodeRouteFeedbackURLEnv          = "OPENCODE_ROUTE_FEEDBACK_URL"
	openCodeRouteFeedbackSecretEnv       = "OPENCODE_ROUTE_FEEDBACK_SECRET"
	openCodeRouteFeedbackTimestampHeader = "X-Route-Feedback-Timestamp"
	openCodeRouteFeedbackSignatureHeader = "X-Route-Feedback-Signature"
)

var openCodeRouteFeedbackHTTPClient = &http.Client{Timeout: 3 * time.Second}

type OpenCodeRouteLeaseRequest struct {
	Service    string `json:"service"`
	RequestID  string `json:"request_id"`
	TTLSeconds int    `json:"ttl_sec"`
}

type OpenCodeRouteLease = relaycommon.OpenCodeRouteLease

type OpenCodeRouteFeedback struct {
	LeaseID             string `json:"lease_id"`
	RequestID           string `json:"request_id"`
	Model               string `json:"model"`
	Outcome             string `json:"outcome"`
	TransportKind       string `json:"transport_kind,omitempty"`
	DurationMS          int64  `json:"duration_ms"`
	ReceivedEvents      int    `json:"received_events"`
	RetryCount          int    `json:"retry_count"`
	RecoveredAfterRetry bool   `json:"recovered_after_retry"`
}

func decodeOpenCodeRouteFeedbackJSON(reader io.Reader, target any) error {
	return common.DecodeJson(reader, target)
}

func openCodeRouteFeedbackConfig() (string, string, bool) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(openCodeRouteFeedbackURLEnv)), "/")
	secret := strings.TrimSpace(os.Getenv(openCodeRouteFeedbackSecretEnv))
	return baseURL, secret, baseURL != "" && secret != ""
}

func signOpenCodeRouteFeedback(secret string, timestamp int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func postOpenCodeRouteFeedback(ctx context.Context, path string, requestBody any, responseBody any) error {
	baseURL, secret, enabled := openCodeRouteFeedbackConfig()
	if !enabled {
		return nil
	}
	payload, err := common.Marshal(requestBody)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	timestamp := time.Now().Unix()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(openCodeRouteFeedbackTimestampHeader, fmt.Sprintf("%d", timestamp))
	request.Header.Set(openCodeRouteFeedbackSignatureHeader, signOpenCodeRouteFeedback(secret, timestamp, payload))
	response, err := openCodeRouteFeedbackHTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("route feedback returned HTTP %d", response.StatusCode)
	}
	if responseBody != nil {
		return common.DecodeJson(response.Body, responseBody)
	}
	return nil
}

func acquireOpenCodeRouteLease(ctx context.Context, requestID string) (*OpenCodeRouteLease, error) {
	_, _, enabled := openCodeRouteFeedbackConfig()
	if !enabled {
		return nil, nil
	}
	var lease OpenCodeRouteLease
	err := postOpenCodeRouteFeedback(ctx, "/api/route-feedback/lease", OpenCodeRouteLeaseRequest{
		Service:    "opencode_go",
		RequestID:  requestID,
		TTLSeconds: 600,
	}, &lease)
	if err != nil {
		return nil, err
	}
	if lease.LeaseID == "" || lease.RouteKey == "" {
		return nil, fmt.Errorf("route feedback returned incomplete lease")
	}
	return &lease, nil
}

func buildOpenCodeRouteFeedback(info *relaycommon.RelayInfo, lease *OpenCodeRouteLease, success bool) *OpenCodeRouteFeedback {
	if info == nil || lease == nil || lease.LeaseID == "" {
		return nil
	}
	feedback := &OpenCodeRouteFeedback{
		LeaseID:             lease.LeaseID,
		RequestID:           info.RequestId,
		Model:               info.OriginModelName,
		Outcome:             "success",
		DurationMS:          time.Since(info.StartTime).Milliseconds(),
		ReceivedEvents:      info.ReceivedResponseCount,
		RetryCount:          info.OpenCodeStreamRetryCount,
		RecoveredAfterRetry: info.OpenCodeRecoveredAfterRetry,
	}
	if lease.AcquiredAt > 0 {
		feedback.DurationMS = max(0, time.Now().UnixMilli()-int64(lease.AcquiredAt*1000))
	}
	if success {
		return feedback
	}
	if info.LastError == nil {
		return nil
	}
	if errorInfo := info.LastError.GetUpstreamErrorInfo(); errorInfo != nil {
		feedback.TransportKind = errorInfo.Kind
	}
	switch feedback.TransportKind {
	case "client_canceled":
		feedback.Outcome = "client_canceled"
	case "upstream_stream_terminated":
		feedback.Outcome = "stream_incomplete"
	case "unexpected_eof", "eof", "tls", "connection_reset", "connection_closed", "timeout", "network", "transport":
		feedback.Outcome = "transport_failure"
	default:
		feedback.Outcome = "service_failure"
	}
	return feedback
}

func reportOpenCodeRouteFeedbackAsync(feedback OpenCodeRouteFeedback) {
	common.RelayCtxGo(context.Background(), func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := postOpenCodeRouteFeedback(ctx, "/api/route-feedback/result", feedback, nil); err != nil {
			common.SysError("OpenCode route feedback failed: " + err.Error())
		}
	})
}

func AcquireOpenCodeRouteLease(ctx context.Context, requestID string) (*OpenCodeRouteLease, error) {
	return acquireOpenCodeRouteLease(ctx, requestID)
}

func ShouldUseOpenCodeRouteFeedback(info *relaycommon.RelayInfo) bool {
	if info == nil || !info.IsStream || strings.TrimSpace(info.ChannelSetting.Proxy) == "" {
		return false
	}
	model := strings.ToLower(strings.TrimSpace(info.OriginModelName))
	baseURL := strings.ToLower(strings.TrimSpace(info.ChannelBaseUrl))
	return strings.Contains(model, "deepseek-v4-flash") && strings.Contains(baseURL, "opencode")
}

func ReportOpenCodeRouteFeedback(info *relaycommon.RelayInfo, success bool) {
	feedback := buildOpenCodeRouteFeedback(info, info.OpenCodeRouteLease, success)
	if feedback != nil {
		reportOpenCodeRouteFeedbackAsync(*feedback)
	}
	info.OpenCodeRouteLease = nil
}

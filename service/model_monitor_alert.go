package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	modelMonitorAlertBatchSize = 20
	modelMonitorAlertLease     = 5 * time.Minute
	modelMonitorAlertMaxTries  = 5
)

var (
	modelMonitorTelegramAPIBaseURL = "https://api.telegram.org"
	modelMonitorAlertHTTPClient    = &http.Client{Timeout: 15 * time.Second}
	sendModelMonitorAlertEmail     = common.SendEmail
	sendModelMonitorAlertTelegram  = SendModelMonitorTelegram
)

type ModelMonitorAlertDispatchSummary struct {
	Claimed  int `json:"claimed"`
	Sent     int `json:"sent"`
	Retrying int `json:"retrying"`
	Dead     int `json:"dead"`
}

type ModelMonitorAlertTransportTestResult struct {
	Enabled bool   `json:"enabled"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type ModelMonitorAlertTestResult struct {
	Email    ModelMonitorAlertTransportTestResult `json:"email"`
	Telegram ModelMonitorAlertTransportTestResult `json:"telegram"`
}

type modelMonitorAlertDeliveryError struct {
	message   string
	retryable bool
}

func (err *modelMonitorAlertDeliveryError) Error() string {
	return err.message
}

func IsModelMonitorAlertRetryable(err error) bool {
	var deliveryError *modelMonitorAlertDeliveryError
	if errors.As(err, &deliveryError) {
		return deliveryError.retryable
	}
	return true
}

func RecordModelMonitorObservation(observation *model.ModelMonitorObservation) error {
	setting := operation_setting.GetModelMonitorAlertSetting()
	transports := setting.EnabledTransports(observation.SiteID, observation.ChannelID, observation.ModelName)
	events, err := model.RecordModelMonitorObservation(observation, transports)
	if err != nil {
		return err
	}
	if len(events) > 0 {
		if _, _, enqueueErr := EnqueueSystemTask(model.SystemTaskTypeModelMonitorAlert, nil); enqueueErr != nil {
			common.SysError("model monitor alert dispatch enqueue failed")
		}
	}
	return nil
}

func SendModelMonitorTelegram(ctx context.Context, token string, chatID string, text string) error {
	token = strings.TrimSpace(token)
	chatID = strings.TrimSpace(chatID)
	if token == "" || chatID == "" {
		return &modelMonitorAlertDeliveryError{message: "Telegram notification is not configured", retryable: false}
	}
	payload, err := common.Marshal(struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		return &modelMonitorAlertDeliveryError{message: "Telegram notification payload is invalid", retryable: false}
	}
	endpoint := strings.TrimRight(modelMonitorTelegramAPIBaseURL, "/") + "/bot" + token + "/sendMessage"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return &modelMonitorAlertDeliveryError{message: "Telegram notification request is invalid", retryable: false}
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := modelMonitorAlertHTTPClient.Do(request)
	if err != nil {
		return &modelMonitorAlertDeliveryError{message: "Telegram notification request failed", retryable: true}
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError
		return &modelMonitorAlertDeliveryError{
			message:   fmt.Sprintf("Telegram notification returned HTTP %d", response.StatusCode),
			retryable: retryable,
		}
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if err := common.DecodeJson(response.Body, &result); err != nil {
		return &modelMonitorAlertDeliveryError{message: "Telegram notification returned invalid JSON", retryable: true}
	}
	if !result.OK {
		return &modelMonitorAlertDeliveryError{message: "Telegram notification was rejected", retryable: false}
	}
	return nil
}

func TestModelMonitorAlertTransports(ctx context.Context) (ModelMonitorAlertTestResult, bool) {
	setting := operation_setting.GetModelMonitorAlertSetting()
	result := ModelMonitorAlertTestResult{
		Email: ModelMonitorAlertTransportTestResult{
			Enabled: setting.EmailEnabled,
			Success: !setting.EmailEnabled,
		},
		Telegram: ModelMonitorAlertTransportTestResult{
			Enabled: setting.TelegramEnabled,
			Success: !setting.TelegramEnabled,
		},
	}
	const subject = "[NewAPI] Model monitor notification test"
	const text = "NewAPI 模型监控通知测试\n邮件或 Telegram 收到此消息，表示通知渠道配置可用。"
	const htmlBody = "NewAPI 模型监控通知测试<br>邮件或 Telegram 收到此消息，表示通知渠道配置可用。"
	if !setting.EmailEnabled && !setting.TelegramEnabled {
		return result, false
	}
	if setting.EmailEnabled {
		if err := sendModelMonitorAlertEmail(subject, setting.EmailRecipients, htmlBody); err != nil {
			result.Email.Error = err.Error()
		} else {
			result.Email.Success = true
		}
	}
	if setting.TelegramEnabled {
		if err := sendModelMonitorAlertTelegram(ctx, setting.TelegramNotifyBotToken, setting.TelegramChatID, text); err != nil {
			result.Telegram.Error = err.Error()
		} else {
			result.Telegram.Success = true
		}
	}
	return result, result.Email.Success && result.Telegram.Success
}

func DispatchDueModelMonitorAlerts(ctx context.Context, runnerID string, now int64) (ModelMonitorAlertDispatchSummary, error) {
	summary := ModelMonitorAlertDispatchSummary{}
	events, err := model.ClaimDueModelMonitorAlertOutbox(
		now,
		now+int64(modelMonitorAlertLease.Seconds()),
		runnerID,
		modelMonitorAlertBatchSize,
	)
	if err != nil {
		return summary, err
	}
	summary.Claimed = len(events)
	for _, event := range events {
		if ctx != nil && ctx.Err() != nil {
			if retryErr := model.RetryModelMonitorAlertOutbox(
				event.ID,
				runnerID,
				now+int64(modelMonitorAlertRetryDelay(event.Attempts).Seconds()),
				"model monitor alert dispatch cancelled",
				modelMonitorAlertMaxTries,
			); retryErr != nil {
				return summary, retryErr
			}
			summary.Retrying++
			continue
		}

		sendErr := deliverModelMonitorAlert(ctx, event)
		if sendErr == nil {
			if err := model.CompleteModelMonitorAlertOutbox(event.ID, runnerID, now); err != nil {
				return summary, err
			}
			summary.Sent++
			continue
		}

		maxAttempts := modelMonitorAlertMaxTries
		if !IsModelMonitorAlertRetryable(sendErr) {
			maxAttempts = event.Attempts
		}
		nextAttemptAt := now + int64(modelMonitorAlertRetryDelay(event.Attempts).Seconds())
		if err := model.RetryModelMonitorAlertOutbox(event.ID, runnerID, nextAttemptAt, sendErr.Error(), maxAttempts); err != nil {
			return summary, err
		}
		if event.Attempts >= maxAttempts {
			summary.Dead++
		} else {
			summary.Retrying++
		}
	}
	return summary, nil
}

func deliverModelMonitorAlert(ctx context.Context, event model.ModelMonitorAlertOutbox) error {
	setting := operation_setting.GetModelMonitorAlertSetting()
	subject, text, htmlBody, err := buildModelMonitorAlertMessage(event)
	if err != nil {
		return &modelMonitorAlertDeliveryError{message: "model monitor alert metadata lookup failed", retryable: true}
	}
	switch event.Transport {
	case model.ModelMonitorAlertTransportEmail:
		if !setting.EmailEnabled || strings.TrimSpace(setting.EmailRecipients) == "" {
			return &modelMonitorAlertDeliveryError{message: "email notification is disabled", retryable: false}
		}
		return sendModelMonitorAlertEmail(subject, setting.EmailRecipients, htmlBody)
	case model.ModelMonitorAlertTransportTelegram:
		if !setting.TelegramEnabled {
			return &modelMonitorAlertDeliveryError{message: "Telegram notification is disabled", retryable: false}
		}
		return sendModelMonitorAlertTelegram(ctx, setting.TelegramNotifyBotToken, setting.TelegramChatID, text)
	default:
		return &modelMonitorAlertDeliveryError{message: "unsupported model monitor alert transport", retryable: false}
	}
}

func buildModelMonitorAlertMessage(event model.ModelMonitorAlertOutbox) (string, string, string, error) {
	siteName := fmt.Sprintf("site #%d", event.SiteID)
	var site model.ModelMonitorSite
	if err := model.DB.Select("name").First(&site, event.SiteID).Error; err == nil {
		siteName = site.Name
	}
	channelName := fmt.Sprintf("channel #%d", event.ChannelID)
	var channel model.Channel
	if err := model.DB.Select("name").First(&channel, event.ChannelID).Error; err == nil {
		channelName = fmt.Sprintf("%s (#%d)", channel.Name, event.ChannelID)
	}

	stateText := "不可用"
	subjectState := "Unavailable"
	if event.Status == model.ModelMonitorStatusAvailable {
		stateText = "已恢复"
		subjectState = "Recovered"
	}
	observedAt := time.Unix(event.ObservedAt, 0).In(time.Local).Format("2006-01-02 15:04:05 MST")
	lines := []string{
		fmt.Sprintf("模型监控状态：%s", stateText),
		fmt.Sprintf("站点：%s", siteName),
		fmt.Sprintf("渠道：%s", channelName),
		fmt.Sprintf("模型：%s", event.ModelName),
		fmt.Sprintf("时间：%s", observedAt),
	}
	if event.Status == model.ModelMonitorStatusUnavailable && event.FailureType != "" {
		lines = append(lines, fmt.Sprintf("失败类型：%s", event.FailureType))
	}
	if summary := strings.TrimSpace(event.ErrorSummary); summary != "" {
		lines = append(lines, fmt.Sprintf("错误：%s", summary))
	}
	text := strings.Join(lines, "\n")
	htmlLines := make([]string, 0, len(lines))
	for _, line := range lines {
		htmlLines = append(htmlLines, html.EscapeString(line))
	}
	subject := fmt.Sprintf("[NewAPI] %s: %s", subjectState, event.ModelName)
	return subject, text, strings.Join(htmlLines, "<br>"), nil
}

func modelMonitorAlertRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	case 3:
		return 10 * time.Minute
	default:
		return 30 * time.Minute
	}
}

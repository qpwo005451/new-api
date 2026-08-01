package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorAlertTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ModelMonitorSite{},
		&model.Channel{},
		&model.ModelMonitorAlertOutbox{},
	))
	previousDB := model.DB
	previousSetting := *operation_setting.GetModelMonitorAlertSetting()
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		*operation_setting.GetModelMonitorAlertSetting() = previousSetting
	})
	return db
}

func TestSendModelMonitorTelegramUsesDedicatedNotificationToken(t *testing.T) {
	var receivedPath string
	var received struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		require.NoError(t, common.DecodeJson(r.Body, &received))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	previousBaseURL := modelMonitorTelegramAPIBaseURL
	previousOAuthToken := common.TelegramBotToken
	modelMonitorTelegramAPIBaseURL = server.URL
	common.TelegramBotToken = "oauth-token-must-not-be-used"
	t.Cleanup(func() {
		modelMonitorTelegramAPIBaseURL = previousBaseURL
		common.TelegramBotToken = previousOAuthToken
	})

	err := SendModelMonitorTelegram(context.Background(), "notify-token", "12345", "alert text")
	require.NoError(t, err)
	assert.Equal(t, "/botnotify-token/sendMessage", receivedPath)
	assert.Equal(t, "12345", received.ChatID)
	assert.Equal(t, "alert text", received.Text)
}

func TestSendModelMonitorTelegramClassifiesHTTPFailures(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "bad request is permanent", status: http.StatusBadRequest, retryable: false},
		{name: "rate limit is retryable", status: http.StatusTooManyRequests, retryable: true},
		{name: "server error is retryable", status: http.StatusBadGateway, retryable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "failed", test.status)
			}))
			defer server.Close()
			previousBaseURL := modelMonitorTelegramAPIBaseURL
			modelMonitorTelegramAPIBaseURL = server.URL
			t.Cleanup(func() { modelMonitorTelegramAPIBaseURL = previousBaseURL })

			err := SendModelMonitorTelegram(context.Background(), "notify-token", "12345", "alert text")
			require.Error(t, err)
			assert.Equal(t, test.retryable, IsModelMonitorAlertRetryable(err))
		})
	}
}

func TestDispatchModelMonitorAlertsKeepsEmailAndTelegramIndependent(t *testing.T) {
	db := setupModelMonitorAlertTestDB(t)
	setting := operation_setting.GetModelMonitorAlertSetting()
	setting.Enabled = true
	setting.EmailEnabled = true
	setting.EmailRecipients = "ops@example.com"
	setting.TelegramEnabled = true
	setting.TelegramNotifyBotToken = "notify-token"
	setting.TelegramChatID = "12345"

	require.NoError(t, db.Create(&[]model.ModelMonitorAlertOutbox{
		{
			EventKey: "email-event", SiteID: 2, TargetID: 7, ChannelID: 9, ModelName: "gpt-5.6-luna",
			Status: model.ModelMonitorStatusUnavailable, Transport: model.ModelMonitorAlertTransportEmail,
			DeliveryStatus: model.ModelMonitorAlertDeliveryPending, NextAttemptAt: 100,
		},
		{
			EventKey: "telegram-event", SiteID: 2, TargetID: 7, ChannelID: 9, ModelName: "gpt-5.6-luna",
			Status: model.ModelMonitorStatusUnavailable, Transport: model.ModelMonitorAlertTransportTelegram,
			DeliveryStatus: model.ModelMonitorAlertDeliveryPending, NextAttemptAt: 100,
		},
	}).Error)

	previousEmailSender := sendModelMonitorAlertEmail
	previousTelegramSender := sendModelMonitorAlertTelegram
	sendModelMonitorAlertEmail = func(_, _, _ string) error {
		return assert.AnError
	}
	telegramCalls := 0
	sendModelMonitorAlertTelegram = func(_ context.Context, token, chatID, _ string) error {
		telegramCalls++
		assert.Equal(t, "notify-token", token)
		assert.Equal(t, "12345", chatID)
		return nil
	}
	t.Cleanup(func() {
		sendModelMonitorAlertEmail = previousEmailSender
		sendModelMonitorAlertTelegram = previousTelegramSender
	})

	summary, err := DispatchDueModelMonitorAlerts(context.Background(), "runner-a", 100)
	require.NoError(t, err)
	assert.Equal(t, 2, summary.Claimed)
	assert.Equal(t, 1, summary.Sent)
	assert.Equal(t, 1, summary.Retrying)
	assert.Equal(t, 1, telegramCalls)

	var email model.ModelMonitorAlertOutbox
	require.NoError(t, db.Where("transport = ?", model.ModelMonitorAlertTransportEmail).First(&email).Error)
	assert.Equal(t, model.ModelMonitorAlertDeliveryPending, email.DeliveryStatus)
	assert.NotEmpty(t, email.LastError)
	var telegram model.ModelMonitorAlertOutbox
	require.NoError(t, db.Where("transport = ?", model.ModelMonitorAlertTransportTelegram).First(&telegram).Error)
	assert.Equal(t, model.ModelMonitorAlertDeliverySent, telegram.DeliveryStatus)
}

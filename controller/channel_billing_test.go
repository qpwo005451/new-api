package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateChannelBalanceFallsBackToSub2APIUsage(t *testing.T) {
	tests := []struct {
		name          string
		channelType   int
		baseURLSuffix string
	}{
		{
			name:        "OpenAI channel with root base URL",
			channelType: constant.ChannelTypeOpenAI,
		},
		{
			name:          "base URL ending in v1",
			channelType:   constant.ChannelTypeOpenAI,
			baseURLSuffix: "/v1",
		},
		{
			name:        "custom channel",
			channelType: constant.ChannelTypeCustom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usageRequests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/usage":
					usageRequests++
					assert.Equal(t, "Bearer sk-sub2api-test", r.Header.Get("Authorization"))
					w.Header().Set("Content-Type", "application/json")
					_, err := fmt.Fprint(w, `{"mode":"unrestricted","isValid":true,"remaining":87.74,"unit":"USD","balance":87.74}`)
					assert.NoError(t, err)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			setupChannelBillingTestDB(t)

			baseURL := server.URL + tt.baseURLSuffix
			channel := &model.Channel{
				Name:    "sub2api",
				Type:    tt.channelType,
				Key:     "sk-sub2api-test",
				BaseURL: &baseURL,
			}
			require.NoError(t, model.DB.Create(channel).Error)

			balance, err := updateChannelBalance(channel)
			require.NoError(t, err)
			assert.Equal(t, 87.74, balance)
			assert.Equal(t, 1, usageRequests)

			var stored model.Channel
			require.NoError(t, model.DB.First(&stored, channel.Id).Error)
			assert.Equal(t, 87.74, stored.Balance)
			assert.NotZero(t, stored.BalanceUpdatedTime)
		})
	}
}

func TestUpdateChannelBalanceKeepsLegacyOpenAIPriority(t *testing.T) {
	var usageRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/dashboard/billing/subscription":
			_, err := fmt.Fprint(w, `{"has_payment_method":true,"hard_limit_usd":100}`)
			assert.NoError(t, err)
		case "/v1/dashboard/billing/usage":
			_, err := fmt.Fprint(w, `{"total_usage":1250}`)
			assert.NoError(t, err)
		case "/v1/usage":
			usageRequests++
			http.Error(w, "sub2api fallback must not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	setupChannelBillingTestDB(t)

	baseURL := server.URL
	channel := &model.Channel{
		Name:    "legacy OpenAI",
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-legacy-test",
		BaseURL: &baseURL,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	balance, err := updateChannelBalance(channel)
	require.NoError(t, err)
	assert.Equal(t, 87.5, balance)
	assert.Zero(t, usageRequests)
}

func TestParseSub2APIUsageBalance(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    float64
		wantErr string
	}{
		{
			name: "wallet balance",
			body: `{"mode":"unrestricted","isValid":true,"remaining":12.5,"unit":"USD","balance":12.5}`,
			want: 12.5,
		},
		{
			name: "key quota",
			body: `{"mode":"quota_limited","isValid":true,"quota":{"limit":100,"used":25,"remaining":75,"unit":"USD"},"remaining":75,"unit":"USD"}`,
			want: 75,
		},
		{
			name: "nested key quota",
			body: `{"mode":"quota_limited","isValid":true,"quota":{"remaining":42.25,"unit":"USD"}}`,
			want: 42.25,
		},
		{
			name: "zero balance remains valid",
			body: `{"mode":"unrestricted","isValid":true,"remaining":0,"unit":"USD","balance":0}`,
			want: 0,
		},
		{
			name:    "invalid key",
			body:    `{"mode":"unrestricted","isValid":false,"remaining":10,"unit":"USD"}`,
			wantErr: "invalid API key",
		},
		{
			name:    "unsupported mode",
			body:    `{"mode":"unknown","isValid":true,"remaining":10,"unit":"USD"}`,
			wantErr: "unsupported sub2api usage mode",
		},
		{
			name:    "unsupported currency",
			body:    `{"mode":"unrestricted","isValid":true,"remaining":10,"unit":"CNY"}`,
			wantErr: "unsupported unit",
		},
		{
			name:    "unlimited quota is not a negative balance",
			body:    `{"mode":"unrestricted","isValid":true,"remaining":-1,"unit":"USD"}`,
			wantErr: "unlimited quota",
		},
		{
			name:    "negative balance",
			body:    `{"mode":"unrestricted","isValid":true,"remaining":-2,"unit":"USD"}`,
			wantErr: "must not be negative",
		},
		{
			name:    "missing balance",
			body:    `{"mode":"unrestricted","isValid":true,"unit":"USD"}`,
			wantErr: "missing remaining balance",
		},
		{
			name:    "malformed JSON",
			body:    `{"mode":`,
			wantErr: "unexpected end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSub2APIUsageBalance([]byte(tt.body))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func setupChannelBillingTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
	})
}

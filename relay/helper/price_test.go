package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestModelPriceHelperUsesInputPricingAliasOnlyForInputChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	savedModelRatio := ratio_setting.ModelRatio2JSONString()
	savedCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedModelRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(savedCompletionRatio))
	})

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.5":0,"input/gpt-5.5":2.5}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-5.5":0,"input/gpt-5.5":6}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("group", "default")

	nonInputInfo := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		UserGroup:       "default",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 8,
		},
	}
	nonInputPrice, err := ModelPriceHelper(ctx, nonInputInfo, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 0.0, nonInputPrice.ModelRatio)
	require.Equal(t, 0, nonInputPrice.QuotaToPreConsume)

	inputInfo := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		UserGroup:       "default",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 9,
		},
	}
	inputPrice, err := ModelPriceHelper(ctx, inputInfo, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 2.5, inputPrice.ModelRatio)
	require.Equal(t, 6.0, inputPrice.CompletionRatio)
	require.Equal(t, int(float64(common.Max(1000, common.PreConsumedQuota))*2.5), inputPrice.QuotaToPreConsume)
	require.Equal(t, "gpt-5.5", inputInfo.OriginModelName)
}

func TestModelPriceHelperUsesInputTieredPricingAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"input/gpt-5.5":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"input/gpt-5.5":"len <= 270000 ? tier(\"standard\", p * 5 + cr * 0.5 + c * 30) : tier(\"long_context\", p * 10 + cr * 1 + c * 45)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("group", "default")

	inputInfo := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		UserGroup:       "default",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 9,
		},
	}
	inputPrice, err := ModelPriceHelper(ctx, inputInfo, 1000, &types.TokenCountMeta{MaxTokens: 100})
	require.NoError(t, err)
	require.Equal(t, 4000, inputPrice.QuotaToPreConsume)
	require.NotNil(t, inputInfo.TieredBillingSnapshot)
	require.Equal(t, "input/gpt-5.5", inputInfo.TieredBillingSnapshot.ModelName)
	require.Equal(t, "standard", inputInfo.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, "gpt-5.5", inputInfo.OriginModelName)
}

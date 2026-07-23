package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

// https://github.com/songquanpeng/one-api/issues/79

type OpenAISubscriptionResponse struct {
	Object             string  `json:"object"`
	HasPaymentMethod   bool    `json:"has_payment_method"`
	SoftLimitUSD       float64 `json:"soft_limit_usd"`
	HardLimitUSD       float64 `json:"hard_limit_usd"`
	SystemHardLimitUSD float64 `json:"system_hard_limit_usd"`
	AccessUntil        int64   `json:"access_until"`
}

type OpenAIUsageDailyCost struct {
	Timestamp float64 `json:"timestamp"`
	LineItems []struct {
		Name string  `json:"name"`
		Cost float64 `json:"cost"`
	}
}

type OpenAICreditGrants struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalAvailable float64 `json:"total_available"`
}

type OpenAIUsageResponse struct {
	Object string `json:"object"`
	//DailyCosts []OpenAIUsageDailyCost `json:"daily_costs"`
	TotalUsage float64 `json:"total_usage"` // unit: 0.01 dollar
}

type OpenAISBUsageResponse struct {
	Msg  string `json:"msg"`
	Data *struct {
		Credit string `json:"credit"`
	} `json:"data"`
}

type AIProxyUserOverviewResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ErrorCode int    `json:"error_code"`
	Data      struct {
		TotalPoints float64 `json:"totalPoints"`
	} `json:"data"`
}

type API2GPTUsageResponse struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalRemaining float64 `json:"total_remaining"`
}

type APGC2DGPTUsageResponse struct {
	//Grants         interface{} `json:"grants"`
	Object         string  `json:"object"`
	TotalAvailable float64 `json:"total_available"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
}

type SiliconFlowUsageResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Data    struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Image         string `json:"image"`
		Email         string `json:"email"`
		IsAdmin       bool   `json:"isAdmin"`
		Balance       string `json:"balance"`
		Status        string `json:"status"`
		Introduction  string `json:"introduction"`
		Role          string `json:"role"`
		ChargeBalance string `json:"chargeBalance"`
		TotalBalance  string `json:"totalBalance"`
		Category      string `json:"category"`
	} `json:"data"`
}

type DeepSeekUsageResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

type OpenRouterCreditResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

type Sub2APIUsageResponse struct {
	Mode         string   `json:"mode"`
	IsValid      bool     `json:"isValid"`
	Remaining    *float64 `json:"remaining"`
	Balance      *float64 `json:"balance"`
	Unit         string   `json:"unit"`
	Subscription *struct {
		DailyLimitUSD   *float64 `json:"daily_limit_usd"`
		DailyUsageUSD   *float64 `json:"daily_usage_usd"`
		WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
		WeeklyUsageUSD  *float64 `json:"weekly_usage_usd"`
		MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
		MonthlyUsageUSD *float64 `json:"monthly_usage_usd"`
	} `json:"subscription"`
	Quota *struct {
		Remaining *float64 `json:"remaining"`
		Unit      string   `json:"unit"`
	} `json:"quota"`
}

// GetAuthHeader get auth header
func GetAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return h
}

// GetClaudeAuthHeader get claude auth header
func GetClaudeAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("x-api-key", token)
	h.Add("anthropic-version", "2023-06-01")
	return h
}

func GetResponseBody(method, url string, channel *model.Channel, headers http.Header) ([]byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	for k := range headers {
		req.Header.Add(k, headers.Get(k))
	}
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	err = res.Body.Close()
	if err != nil {
		return nil, err
	}
	return body, nil
}

func updateChannelCloseAIBalance(channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("%s/dashboard/billing/credit_grants", channel.GetBaseURL())
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := OpenAICreditGrants{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalAvailable)
	return response.TotalAvailable, nil
}

func updateChannelOpenAISBBalance(channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("https://api.openai-sb.com/sb-api/user/status?api_key=%s", channel.Key)
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenAISBUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Data == nil {
		return 0, errors.New(response.Msg)
	}
	balance, err := strconv.ParseFloat(response.Data.Credit, 64)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelAIProxyBalance(channel *model.Channel) (float64, error) {
	url := "https://aiproxy.io/api/report/getUserOverview"
	headers := http.Header{}
	headers.Add("Api-Key", channel.Key)
	body, err := GetResponseBody("GET", url, channel, headers)
	if err != nil {
		return 0, err
	}
	response := AIProxyUserOverviewResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Success {
		return 0, fmt.Errorf("code: %d, message: %s", response.ErrorCode, response.Message)
	}
	channel.UpdateBalance(response.Data.TotalPoints)
	return response.Data.TotalPoints, nil
}

func updateChannelAPI2GPTBalance(channel *model.Channel) (float64, error) {
	url := "https://api.api2gpt.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := API2GPTUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalRemaining)
	return response.TotalRemaining, nil
}

func updateChannelSiliconFlowBalance(channel *model.Channel) (float64, error) {
	url := "https://api.siliconflow.cn/v1/user/info"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := SiliconFlowUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Code != 20000 {
		return 0, fmt.Errorf("code: %d, message: %s", response.Code, response.Message)
	}
	balance, err := strconv.ParseFloat(response.Data.TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelDeepSeekBalance(channel *model.Channel) (float64, error) {
	url := "https://api.deepseek.com/user/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := DeepSeekUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	index := -1
	for i, balanceInfo := range response.BalanceInfos {
		if balanceInfo.Currency == "CNY" {
			index = i
			break
		}
	}
	if index == -1 {
		return 0, errors.New("currency CNY not found")
	}
	balance, err := strconv.ParseFloat(response.BalanceInfos[index].TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelAIGC2DBalance(channel *model.Channel) (float64, error) {
	url := "https://api.aigc2d.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := APGC2DGPTUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalAvailable)
	return response.TotalAvailable, nil
}

func updateChannelOpenRouterBalance(channel *model.Channel) (float64, error) {
	url := "https://openrouter.ai/api/v1/credits"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenRouterCreditResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	balance := response.Data.TotalCredits - response.Data.TotalUsage
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelMoonshotBalance(channel *model.Channel) (float64, error) {
	url := "https://api.moonshot.cn/v1/users/me/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}

	type MoonshotBalanceData struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	}

	type MoonshotBalanceResponse struct {
		Code   int                 `json:"code"`
		Data   MoonshotBalanceData `json:"data"`
		Scode  string              `json:"scode"`
		Status bool                `json:"status"`
	}

	response := MoonshotBalanceResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Status || response.Code != 0 {
		return 0, fmt.Errorf("failed to update moonshot balance, status: %v, code: %d, scode: %s", response.Status, response.Code, response.Scode)
	}
	availableBalanceCny := response.Data.AvailableBalance
	availableBalanceUsd := decimal.NewFromFloat(availableBalanceCny).Div(decimal.NewFromFloat(operation_setting.Price)).InexactFloat64()
	channel.UpdateBalance(availableBalanceUsd)
	return availableBalanceUsd, nil
}

func parseSub2APIUsageBalance(body []byte) (float64, error) {
	response := Sub2APIUsageResponse{}
	if err := common.Unmarshal(body, &response); err != nil {
		return 0, err
	}
	if response.Mode != "quota_limited" && response.Mode != "unrestricted" {
		return 0, fmt.Errorf("unsupported sub2api usage mode: %s", response.Mode)
	}
	if !response.IsValid {
		return 0, errors.New("sub2api reports invalid API key")
	}

	unit := response.Unit
	if unit == "" && response.Quota != nil {
		unit = response.Quota.Unit
	}
	if !strings.EqualFold(unit, "USD") {
		return 0, fmt.Errorf("sub2api uses unsupported unit: %s", unit)
	}

	if response.Mode == "unrestricted" && response.Subscription != nil {
		effectiveRemaining := math.Inf(1)
		hasSubscriptionLimit := false
		windows := []struct {
			name  string
			limit *float64
			usage *float64
		}{
			{name: "daily", limit: response.Subscription.DailyLimitUSD, usage: response.Subscription.DailyUsageUSD},
			{name: "weekly", limit: response.Subscription.WeeklyLimitUSD, usage: response.Subscription.WeeklyUsageUSD},
			{name: "monthly", limit: response.Subscription.MonthlyLimitUSD, usage: response.Subscription.MonthlyUsageUSD},
		}
		for _, window := range windows {
			if window.limit == nil || *window.limit == 0 {
				continue
			}
			if math.IsNaN(*window.limit) || math.IsInf(*window.limit, 0) || *window.limit < 0 {
				return 0, fmt.Errorf("sub2api subscription %s limit must be a non-negative finite number", window.name)
			}
			if window.usage == nil {
				return 0, fmt.Errorf("sub2api subscription %s usage is missing", window.name)
			}
			if math.IsNaN(*window.usage) || math.IsInf(*window.usage, 0) || *window.usage < 0 {
				return 0, fmt.Errorf("sub2api subscription %s usage must be a non-negative finite number", window.name)
			}
			remaining := max(*window.limit-*window.usage, 0)
			effectiveRemaining = min(effectiveRemaining, remaining)
			hasSubscriptionLimit = true
		}
		if hasSubscriptionLimit {
			return effectiveRemaining, nil
		}
	}

	balance := response.Remaining
	if balance == nil {
		balance = response.Balance
	}
	if balance == nil && response.Quota != nil {
		balance = response.Quota.Remaining
	}
	if balance == nil {
		return 0, errors.New("sub2api response is missing remaining balance")
	}
	if math.IsNaN(*balance) || math.IsInf(*balance, 0) {
		return 0, errors.New("sub2api balance must be finite")
	}
	if *balance == -1 {
		return 0, errors.New("sub2api reports unlimited quota")
	}
	if *balance < 0 {
		return 0, fmt.Errorf("sub2api balance must not be negative: %v", *balance)
	}
	return *balance, nil
}

func updateChannelSub2APIBalance(channel *model.Channel, baseURL string) (float64, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	if baseURL == "" {
		return 0, errors.New("sub2api base URL is empty")
	}

	body, err := GetResponseBody(
		http.MethodGet,
		fmt.Sprintf("%s/v1/usage", baseURL),
		channel,
		GetAuthHeader(channel.Key),
	)
	if err != nil {
		return 0, err
	}
	balance, err := parseSub2APIUsageBalance(body)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelLegacyOpenAIBalance(channel *model.Channel, baseURL string) (float64, error) {
	url := fmt.Sprintf("%s/v1/dashboard/billing/subscription", baseURL)
	body, err := GetResponseBody(http.MethodGet, url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	subscription := OpenAISubscriptionResponse{}
	if err = common.Unmarshal(body, &subscription); err != nil {
		return 0, err
	}

	now := time.Now()
	startDate := fmt.Sprintf("%s-01", now.Format("2006-01"))
	endDate := now.Format("2006-01-02")
	if !subscription.HasPaymentMethod {
		startDate = now.AddDate(0, 0, -100).Format("2006-01-02")
	}
	url = fmt.Sprintf("%s/v1/dashboard/billing/usage?start_date=%s&end_date=%s", baseURL, startDate, endDate)
	body, err = GetResponseBody(http.MethodGet, url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	usage := OpenAIUsageResponse{}
	if err = common.Unmarshal(body, &usage); err != nil {
		return 0, err
	}

	balance := subscription.HardLimitUSD - usage.TotalUsage/100
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelBalance(channel *model.Channel) (float64, error) {
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() == "" {
		channel.BaseURL = &baseURL
	}
	switch channel.Type {
	case constant.ChannelTypeOpenAI:
		if channel.GetBaseURL() != "" {
			baseURL = channel.GetBaseURL()
		}
	case constant.ChannelTypeAzure:
		return 0, errors.New("尚未实现")
	case constant.ChannelTypeCustom:
		baseURL = channel.GetBaseURL()
	//case common.ChannelTypeOpenAISB:
	//	return updateChannelOpenAISBBalance(channel)
	case constant.ChannelTypeAIProxy:
		return updateChannelAIProxyBalance(channel)
	case constant.ChannelTypeAPI2GPT:
		return updateChannelAPI2GPTBalance(channel)
	case constant.ChannelTypeAIGC2D:
		return updateChannelAIGC2DBalance(channel)
	case constant.ChannelTypeSiliconFlow:
		return updateChannelSiliconFlowBalance(channel)
	case constant.ChannelTypeDeepSeek:
		return updateChannelDeepSeekBalance(channel)
	case constant.ChannelTypeOpenRouter:
		return updateChannelOpenRouterBalance(channel)
	case constant.ChannelTypeMoonshot:
		return updateChannelMoonshotBalance(channel)
	default:
		return 0, errors.New("尚未实现")
	}

	balance, legacyErr := updateChannelLegacyOpenAIBalance(channel, baseURL)
	if legacyErr == nil {
		return balance, nil
	}
	balance, sub2APIErr := updateChannelSub2APIBalance(channel, baseURL)
	if sub2APIErr == nil {
		return balance, nil
	}
	return 0, fmt.Errorf(
		"legacy OpenAI balance query failed: %v; sub2api balance query failed: %w",
		legacyErr,
		sub2APIErr,
	)
}

func UpdateChannelBalance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "多密钥渠道不支持余额查询",
		})
		return
	}
	balance, err := checkChannelBalanceWithProtection(channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	attachChannelBalanceProtection(channel)
	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"message":            "",
		"balance":            balance,
		"balance_protection": channel.BalanceProtection,
	})
}

func updateAllChannelsBalance(includeProtected bool) error {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if channel.ChannelInfo.IsMultiKey {
			continue // skip multi-key channels
		}
		protection, protectionErr := model.GetChannelBalanceProtection(channel.Id)
		if protectionErr != nil {
			continue
		}
		if protection != nil && protection.Enabled && !includeProtected {
			continue
		}
		// TODO: support Azure
		//if channel.Type != common.ChannelTypeOpenAI && channel.Type != common.ChannelTypeCustom {
		//	continue
		//}
		balance, err := checkChannelBalanceWithProtection(channel)
		if err != nil {
			continue
		} else {
			if balance <= 0 && (protection == nil || !protection.Enabled) {
				service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan()), "余额不足")
			}
		}
		time.Sleep(common.RequestInterval)
	}
	return nil
}

func UpdateAllChannelsBalance(c *gin.Context) {
	// TODO: make it async
	err := updateAllChannelsBalance(true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func AutomaticallyUpdateChannels(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Minute)
		common.SysLog("updating all channels")
		_ = updateAllChannelsBalance(false)
		common.SysLog("channels update done")
	}
}

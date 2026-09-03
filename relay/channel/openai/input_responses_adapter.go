package openai

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

const (
	inputAPIHost              = "ai.input.im"
	inputDeepSeekV4FlashModel = "deepseek-v4-flash"
)

func isInputDeepSeekV4FlashResponsesRequest(info *relaycommon.RelayInfo, model string) bool {
	if info == nil || info.RelayMode != relayconstant.RelayModeResponses {
		return false
	}
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return false
	}
	if info.ChannelType != constant.ChannelTypeOpenAI || !isInputBaseURL(info.ChannelBaseUrl) {
		return false
	}
	if strings.TrimSpace(info.UpstreamModelName) != inputDeepSeekV4FlashModel {
		return false
	}
	return strings.TrimSpace(model) == "" || strings.TrimSpace(model) == inputDeepSeekV4FlashModel
}

func shouldUseInputDeepSeekV4FlashResponsesUpstream(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	return isInputDeepSeekV4FlashResponsesRequest(info, info.UpstreamModelName)
}

func isInputBaseURL(rawBaseURL string) bool {
	baseURL := strings.TrimSpace(rawBaseURL)
	if baseURL == "" {
		return false
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		parsed, err = url.Parse("//" + baseURL)
		if err != nil {
			return false
		}
	}
	return strings.EqualFold(parsed.Hostname(), inputAPIHost)
}

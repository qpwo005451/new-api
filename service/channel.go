package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	if channelError.IsMultiKey && channelError.UsingKey != "" {
		threshold := MultiKeyFailureThreshold()
		if threshold > 0 {
			result, err := model.RecordMultiKeyFailure(channelError.ChannelId, channelError.UsingKey, threshold, reason)
			if err != nil {
				common.SysError(fmt.Sprintf("failed to record multi-key failure: channel_id=%d, error=%v", channelError.ChannelId, err))
				return
			}
			if !result.KeyAutoDisabled {
				common.SysLog(fmt.Sprintf("通道「%s」（#%d）密钥失败计数 %d/%d，暂不禁用", channelError.ChannelName, channelError.ChannelId, result.FailureCount, threshold))
				return
			}
			subject := fmt.Sprintf("通道「%s」（#%d）密钥已被自动禁用", channelError.ChannelName, channelError.ChannelId)
			content := fmt.Sprintf("通道「%s」（#%d）密钥连续失败 %d 次，原因：%s", channelError.ChannelName, channelError.ChannelId, result.FailureCount, reason)
			NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
			return
		}
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func MultiKeyFailureThreshold() int {
	threshold := common.GetEnvOrDefault("MULTI_KEY_FAILURE_THRESHOLD", 0)
	if threshold < 1 {
		return 0
	}
	return threshold
}

func MultiKeyRecoveryIntervalMinutes() int {
	minutes := common.GetEnvOrDefault("MULTI_KEY_RECOVERY_INTERVAL_MINUTES", 0)
	if minutes < 1 {
		return 0
	}
	return minutes
}

func ShouldTrackMultiKeyFailure(err *types.NewAPIError) bool {
	if MultiKeyFailureThreshold() < 1 || err == nil {
		return false
	}

	switch err.GetErrorCode() {
	case types.ErrorCodeInvalidRequest,
		types.ErrorCodeSensitiveWordsDetected,
		types.ErrorCodeViolationFeeGrokCSAM,
		types.ErrorCodeRequestCancelled,
		types.ErrorCodeCountTokenFailed,
		types.ErrorCodeModelPriceError,
		types.ErrorCodeInvalidApiType,
		types.ErrorCodeJsonMarshalFailed,
		types.ErrorCodeGetChannelFailed,
		types.ErrorCodeGenRelayInfoFailed,
		types.ErrorCodeChannelParamOverrideInvalid,
		types.ErrorCodeChannelHeaderOverrideInvalid,
		types.ErrorCodeChannelModelMappedError,
		types.ErrorCodeReadRequestBodyFailed,
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeAccessDenied,
		types.ErrorCodeBadRequestBody,
		types.ErrorCodePromptBlocked,
		types.ErrorCodeInsufficientUserQuota,
		types.ErrorCodePreConsumeTokenQuotaFailed:
		return false
	}
	if types.IsChannelError(err) {
		return true
	}

	switch err.StatusCode {
	case 401, 403, 408, 429:
		return true
	}
	if err.StatusCode >= 500 && err.StatusCode <= 599 {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	for _, marker := range []string{
		"api key not valid",
		"invalid api key",
		"invalid_api_key",
	} {
		if strings.Contains(lowerMessage, marker) {
			return true
		}
	}
	return shouldDisableChannelByError(err)
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	return shouldDisableChannelByError(err)
}

func shouldDisableChannelByError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}

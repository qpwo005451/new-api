package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

type modelMonitorPassiveObservationInput struct {
	ChannelID           int
	ModelName           string
	UpstreamModelName   string
	UpstreamRequestID   string
	Status              string
	FailureType         string
	ErrorSummary        string
	FirstResponseMS     *int64
	TotalDurationMS     int64
	PromptTokens        int
	CompletionTokens    int
	CacheReadTokens     int
	CacheCreationTokens int
	ActualCostUSD       *float64
	ObservedAt          int64
}

func RecordModelMonitorPassiveSuccess(info *relaycommon.RelayInfo, usage *dto.Usage) error {
	if !operation_setting.GetModelMonitorSetting().Enabled {
		return nil
	}
	input, err := newModelMonitorPassiveSuccessInput(info, usage)
	if err != nil {
		return err
	}
	return persistModelMonitorPassiveObservations(input)
}

func RecordModelMonitorPassiveHTTPFailure(info *relaycommon.RelayInfo, statusCode int) error {
	if !operation_setting.GetModelMonitorSetting().Enabled {
		return nil
	}
	input, err := newModelMonitorPassiveFailureInput(info, statusCode)
	if err != nil {
		return err
	}
	return persistModelMonitorPassiveObservations(input)
}

func RecordModelMonitorPassiveSuccessAsync(info *relaycommon.RelayInfo, usage *dto.Usage) {
	if !operation_setting.GetModelMonitorSetting().Enabled {
		return
	}
	input, err := newModelMonitorPassiveSuccessInput(info, usage)
	if err != nil {
		common.SysError("model monitor passive success observation preparation failed")
		return
	}
	gopool.Go(func() {
		if err := persistModelMonitorPassiveObservations(input); err != nil {
			common.SysError("model monitor passive success observation persistence failed")
		}
	})
}

func RecordModelMonitorPassiveHTTPFailureAsync(info *relaycommon.RelayInfo, statusCode int) {
	if !operation_setting.GetModelMonitorSetting().Enabled {
		return
	}
	input, err := newModelMonitorPassiveFailureInput(info, statusCode)
	if err != nil {
		common.SysError("model monitor passive failure observation preparation failed")
		return
	}
	gopool.Go(func() {
		if err := persistModelMonitorPassiveObservations(input); err != nil {
			common.SysError("model monitor passive failure observation persistence failed")
		}
	})
}

func newModelMonitorPassiveSuccessInput(info *relaycommon.RelayInfo, usage *dto.Usage) (modelMonitorPassiveObservationInput, error) {
	input, err := newModelMonitorPassiveObservationInput(info)
	if err != nil {
		return input, err
	}
	input.Status = model.ModelMonitorStatusAvailable
	input.FailureType = model.ModelMonitorFailureTypeNone
	if usage != nil {
		input.PromptTokens = usage.PromptTokens
		input.CompletionTokens = usage.CompletionTokens
		input.CacheReadTokens = usage.PromptTokensDetails.CachedTokens
		input.CacheCreationTokens = usage.PromptTokensDetails.CacheCreationTokensTotal()
		if actualCostUSD, ok := usage.Cost.(float64); ok {
			input.ActualCostUSD = &actualCostUSD
		}
	}
	if info.HasSendResponse() {
		firstResponseMS := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
		if firstResponseMS >= 0 {
			input.FirstResponseMS = &firstResponseMS
		}
	}
	return input, nil
}

func newModelMonitorPassiveFailureInput(info *relaycommon.RelayInfo, statusCode int) (modelMonitorPassiveObservationInput, error) {
	input, err := newModelMonitorPassiveObservationInput(info)
	if err != nil {
		return input, err
	}
	switch {
	case statusCode == http.StatusTooManyRequests || statusCode == http.StatusPaymentRequired:
		input.Status = model.ModelMonitorStatusLimited
		input.FailureType = model.ModelMonitorFailureTypeRateLimited
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		input.Status = model.ModelMonitorStatusUnavailable
		input.FailureType = model.ModelMonitorFailureTypeUnauthorized
	case statusCode == http.StatusNotFound:
		input.Status = model.ModelMonitorStatusUnavailable
		input.FailureType = model.ModelMonitorFailureTypeModelNotFound
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		input.Status = model.ModelMonitorStatusUnavailable
		input.FailureType = model.ModelMonitorFailureTypeTimeout
	case statusCode >= http.StatusInternalServerError:
		input.Status = model.ModelMonitorStatusUnavailable
		input.FailureType = model.ModelMonitorFailureTypeUpstreamServer
	default:
		input.Status = model.ModelMonitorStatusUnavailable
		input.FailureType = model.ModelMonitorFailureTypeConnection
	}
	input.ErrorSummary = fmt.Sprintf("upstream returned HTTP %d", statusCode)
	return input, nil
}

func newModelMonitorPassiveObservationInput(info *relaycommon.RelayInfo) (modelMonitorPassiveObservationInput, error) {
	if info == nil {
		return modelMonitorPassiveObservationInput{}, errors.New("model monitor passive observation relay info is required")
	}
	if info.ChannelId <= 0 {
		return modelMonitorPassiveObservationInput{}, errors.New("model monitor passive observation channel is required")
	}
	if info.StartTime.IsZero() {
		return modelMonitorPassiveObservationInput{}, errors.New("model monitor passive observation start time is required")
	}
	modelName := strings.TrimSpace(info.OriginModelName)
	if modelName == "" {
		return modelMonitorPassiveObservationInput{}, errors.New("model monitor passive observation model is required")
	}

	totalDurationMS := time.Since(info.StartTime).Milliseconds()
	if totalDurationMS < 0 {
		totalDurationMS = 0
	}
	return modelMonitorPassiveObservationInput{
		ChannelID:         info.ChannelId,
		ModelName:         modelName,
		UpstreamModelName: strings.TrimSpace(info.UpstreamModelName),
		UpstreamRequestID: strings.TrimSpace(info.UpstreamRequestId),
		TotalDurationMS:   totalDurationMS,
		ObservedAt:        common.GetTimestamp(),
	}, nil
}

func persistModelMonitorPassiveObservations(input modelMonitorPassiveObservationInput) error {
	paths, err := model.ListEnabledModelMonitorPassivePaths(input.ChannelID, input.ModelName)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}

	observations := make([]model.ModelMonitorObservation, 0, len(paths))
	for _, path := range paths {
		observations = append(observations, model.ModelMonitorObservation{
			SiteID:              path.SiteID,
			TargetID:            path.TargetID,
			ChannelID:           path.ChannelID,
			ModelName:           path.ModelName,
			UpstreamModelName:   input.UpstreamModelName,
			UpstreamRequestID:   input.UpstreamRequestID,
			Status:              input.Status,
			Source:              model.ModelMonitorObservationSourcePassive,
			FailureType:         input.FailureType,
			ErrorSummary:        input.ErrorSummary,
			FirstResponseMS:     input.FirstResponseMS,
			TotalDurationMS:     input.TotalDurationMS,
			PromptTokens:        input.PromptTokens,
			CompletionTokens:    input.CompletionTokens,
			CacheReadTokens:     input.CacheReadTokens,
			CacheCreationTokens: input.CacheCreationTokens,
			CostKind:            model.ModelMonitorCostKindUnknown,
			ObservedAt:          input.ObservedAt,
		})
		observation := &observations[len(observations)-1]
		if input.ActualCostUSD != nil {
			if err := ApplyModelMonitorActualCost(observation, *input.ActualCostUSD); err != nil {
				return err
			}
		} else {
			if err := ApplyModelMonitorEstimatedCost(observation); err != nil {
				return err
			}
		}
	}
	return model.DB.Create(&observations).Error
}

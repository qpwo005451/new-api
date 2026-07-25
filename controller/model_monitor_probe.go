package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const modelMonitorProbeDefaultTimeout = 30 * time.Second

func runModelMonitorProbe(ctx context.Context, channel *model.Channel, target model.ModelMonitorTarget) (model.ModelMonitorObservation, error) {
	if channel == nil {
		return model.ModelMonitorObservation{}, errors.New("model monitor probe channel is required")
	}
	if err := model.ValidateModelMonitorTarget(target); err != nil {
		return model.ModelMonitorObservation{}, err
	}

	endpointType := constant.EndpointType(strings.TrimSpace(target.EndpointType))
	endpointInfo, ok := common.GetDefaultEndpointInfo(endpointType)
	if !ok {
		return model.ModelMonitorObservation{}, fmt.Errorf("unsupported model monitor probe endpoint: %s", endpointType)
	}
	relayFormat, err := modelMonitorProbeRelayFormat(endpointType)
	if err != nil {
		return model.ModelMonitorObservation{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, modelMonitorProbeDefaultTimeout)
		defer cancel()
	}

	observation := model.ModelMonitorObservation{
		SiteID:            target.SiteID,
		ChannelID:         channel.Id,
		TargetID:          target.ID,
		ModelName:         target.ModelName,
		UpstreamModelName: target.ModelName,
		Source:            model.ModelMonitorObservationSourceActive,
		Status:            model.ModelMonitorStatusUnknown,
		FailureType:       model.ModelMonitorFailureTypeNone,
		CostKind:          model.ModelMonitorCostKindUnknown,
		ObservedAt:        common.GetTimestamp(),
	}

	probeChannel, err := copyModelMonitorProbeChannel(channel)
	if err != nil {
		return observation, err
	}
	request := buildTestRequest(target.ModelName, string(endpointType), probeChannel, true)
	recorder := httptest.NewRecorder()
	probeContext, _ := gin.CreateTestContext(recorder)
	probeContext.Request = httptest.NewRequestWithContext(ctx, endpointInfo.Method, endpointInfo.Path, nil)
	probeContext.Request.Header.Set("Content-Type", "application/json")

	if newAPIError := middleware.SetupContextForSelectedChannel(probeContext, probeChannel, target.ModelName); newAPIError != nil {
		return observation, newAPIError
	}

	info, err := relaycommon.GenRelayInfo(probeContext, relayFormat, request, nil)
	if err != nil {
		return observation, err
	}
	info.DisablePing = true
	info.InitChannelMeta(probeContext)
	if err = helper.ModelMappedHelper(probeContext, info, request); err != nil {
		return observation, err
	}
	observation.UpstreamModelName = info.UpstreamModelName

	adaptor, err := prepareModelMonitorProbeAdaptor(probeContext, info, request, relayFormat)
	if err != nil {
		return observation, err
	}

	startedAt := time.Now()
	responseAny, err := adaptor.DoRequest(probeContext, info, probeContext.Request.Body)
	if err != nil {
		observation.TotalDurationMS = time.Since(startedAt).Milliseconds()
		setModelMonitorProbeRequestFailure(&observation, ctx, err)
		return observation, nil
	}

	response, ok := responseAny.(*http.Response)
	if !ok || response == nil {
		observation.TotalDurationMS = time.Since(startedAt).Milliseconds()
		setModelMonitorProbeFailure(&observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeConnection, "upstream did not return an HTTP response")
		return observation, nil
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		observation.TotalDurationMS = time.Since(startedAt).Milliseconds()
		setModelMonitorProbeHTTPFailure(&observation, response.StatusCode)
		return observation, nil
	}

	return consumeModelMonitorProbeStream(ctx, response.Body, startedAt, observation), nil
}

func copyModelMonitorProbeChannel(channel *model.Channel) (*model.Channel, error) {
	probeChannel := *channel
	if !probeChannel.ChannelInfo.IsMultiKey {
		return &probeChannel, nil
	}

	keys := probeChannel.GetKeys()
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil, errors.New("model monitor probe channel has no key")
	}
	probeChannel.Key = strings.TrimSpace(keys[0])
	probeChannel.ChannelInfo.IsMultiKey = false
	return &probeChannel, nil
}

func modelMonitorProbeRelayFormat(endpointType constant.EndpointType) (types.RelayFormat, error) {
	switch endpointType {
	case constant.EndpointTypeOpenAI:
		return types.RelayFormatOpenAI, nil
	case constant.EndpointTypeOpenAIResponse:
		return types.RelayFormatOpenAIResponses, nil
	case constant.EndpointTypeAnthropic:
		return types.RelayFormatClaude, nil
	case constant.EndpointTypeGemini:
		return types.RelayFormatGemini, nil
	default:
		return "", fmt.Errorf("model monitor probe only supports streaming text endpoints, got %s", endpointType)
	}
}

func prepareModelMonitorProbeAdaptor(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request, relayFormat types.RelayFormat) (relaychannel.Adaptor, error) {
	apiType, _ := common.ChannelType2APIType(info.ChannelType)
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return nil, fmt.Errorf("model monitor probe adaptor is unavailable for API type %d", apiType)
	}
	adaptor.Init(info)

	var (
		convertedRequest any
		err              error
	)
	switch relayFormat {
	case types.RelayFormatOpenAIResponses:
		responseRequest, ok := request.(*dto.OpenAIResponsesRequest)
		if !ok {
			return nil, errors.New("model monitor probe request is not a Responses request")
		}
		convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *responseRequest)
	default:
		openAIRequest, ok := request.(*dto.GeneralOpenAIRequest)
		if !ok {
			return nil, errors.New("model monitor probe request is not a text request")
		}
		convertedRequest, err = adaptor.ConvertOpenAIRequest(c, info, openAIRequest)
	}
	if err != nil {
		return nil, fmt.Errorf("convert model monitor probe request failed: %w", err)
	}

	requestData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, fmt.Errorf("marshal model monitor probe request failed: %w", err)
	}
	if len(info.ParamOverride) > 0 {
		requestData, err = relaycommon.ApplyParamOverrideWithRelayInfo(requestData, info)
		if err != nil {
			return nil, fmt.Errorf("apply model monitor probe parameter override failed: %w", err)
		}
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestData))
	return adaptor, nil
}

func consumeModelMonitorProbeStream(ctx context.Context, body io.ReadCloser, startedAt time.Time, observation model.ModelMonitorObservation) model.ModelMonitorObservation {
	defer func() {
		_ = body.Close()
	}()

	scanner := helper.NewStreamScanner(body)
	firstResponseSeen := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			observation.TotalDurationMS = time.Since(startedAt).Milliseconds()
			if !firstResponseSeen {
				setModelMonitorProbeFailure(&observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeInvalidStream, "stream completed without an event")
				return observation
			}
			observation.Status = model.ModelMonitorStatusAvailable
			observation.FailureType = model.ModelMonitorFailureTypeNone
			observation.ErrorSummary = ""
			return observation
		}
		if err := detectErrorFromTestResponseBody([]byte(payload)); err != nil {
			observation.TotalDurationMS = time.Since(startedAt).Milliseconds()
			setModelMonitorProbeStreamError(&observation, err)
			return observation
		}
		if !firstResponseSeen {
			firstResponseMS := time.Since(startedAt).Milliseconds()
			observation.FirstResponseMS = &firstResponseMS
			firstResponseSeen = true
		}
	}

	observation.TotalDurationMS = time.Since(startedAt).Milliseconds()
	if err := scanner.Err(); err != nil {
		setModelMonitorProbeRequestFailure(&observation, ctx, err)
		return observation
	}
	if err := ctx.Err(); err != nil {
		setModelMonitorProbeRequestFailure(&observation, ctx, err)
		return observation
	}
	setModelMonitorProbeFailure(&observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeStreamBreak, "stream ended before completion")
	return observation
}

func setModelMonitorProbeHTTPFailure(observation *model.ModelMonitorObservation, statusCode int) {
	switch {
	case statusCode == http.StatusTooManyRequests:
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusLimited, model.ModelMonitorFailureTypeRateLimited, "upstream returned HTTP 429")
	case statusCode == http.StatusPaymentRequired:
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusLimited, model.ModelMonitorFailureTypeRateLimited, "upstream returned HTTP 402")
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeUnauthorized, fmt.Sprintf("upstream returned HTTP %d", statusCode))
	case statusCode == http.StatusNotFound:
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeModelNotFound, "upstream returned HTTP 404")
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeTimeout, fmt.Sprintf("upstream returned HTTP %d", statusCode))
	case statusCode >= http.StatusInternalServerError:
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeUpstreamServer, fmt.Sprintf("upstream returned HTTP %d", statusCode))
	default:
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeConnection, fmt.Sprintf("upstream returned HTTP %d", statusCode))
	}
}

func setModelMonitorProbeRequestFailure(observation *model.ModelMonitorObservation, ctx context.Context, err error) {
	if errors.Is(err, context.DeadlineExceeded) || (ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeTimeout, "model monitor probe timed out")
		return
	}
	if errors.Is(err, context.Canceled) || (ctx != nil && errors.Is(ctx.Err(), context.Canceled)) {
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnknown, model.ModelMonitorFailureTypeCancelled, "model monitor probe was cancelled")
		return
	}
	setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeConnection, "model monitor probe connection failed")
}

func setModelMonitorProbeStreamError(observation *model.ModelMonitorObservation, err error) {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "rate limit") || strings.Contains(message, "too many requests") || strings.Contains(message, "quota") {
		setModelMonitorProbeFailure(observation, model.ModelMonitorStatusLimited, model.ModelMonitorFailureTypeRateLimited, "upstream rejected the stream")
		return
	}
	setModelMonitorProbeFailure(observation, model.ModelMonitorStatusUnavailable, model.ModelMonitorFailureTypeStreamBreak, "upstream returned an error event")
}

func setModelMonitorProbeFailure(observation *model.ModelMonitorObservation, status string, failureType string, summary string) {
	observation.Status = status
	observation.FailureType = failureType
	observation.ErrorSummary = summary
}

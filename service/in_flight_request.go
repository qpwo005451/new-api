package service

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

var ErrInFlightRequestCancelled = errors.New("request cancelled by administrator; retry the request")

type inFlightRequestEntry struct {
	cancel context.CancelCauseFunc
}

var inFlightRequestRegistry sync.Map

func RegisterInFlightRequest(c *gin.Context, pendingLogId int) {
	if c == nil || c.Request == nil || pendingLogId <= 0 {
		return
	}

	ctx, cancel := context.WithCancelCause(c.Request.Context())
	c.Set(string(inFlightRequestContextKeyName), ctx)
	inFlightRequestRegistry.Store(pendingLogId, &inFlightRequestEntry{cancel: cancel})
}

func UnregisterInFlightRequest(pendingLogId int) {
	if pendingLogId <= 0 {
		return
	}
	inFlightRequestRegistry.Delete(pendingLogId)
}

func CancelInFlightRequest(pendingLogId int) bool {
	entryValue, ok := inFlightRequestRegistry.Load(pendingLogId)
	if !ok {
		return false
	}
	entry, ok := entryValue.(*inFlightRequestEntry)
	if !ok || entry == nil || entry.cancel == nil {
		return false
	}
	entry.cancel(ErrInFlightRequestCancelled)
	return true
}

func RelayRequestContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	ctxValue, ok := c.Get(string(inFlightRequestContextKeyName))
	if !ok {
		return c.Request.Context()
	}
	ctx, ok := ctxValue.(context.Context)
	if !ok || ctx == nil {
		return c.Request.Context()
	}
	return ctx
}

func IsInFlightRequestCancelled(c *gin.Context) bool {
	return errors.Is(context.Cause(RelayRequestContext(c)), ErrInFlightRequestCancelled)
}

func NewInFlightRequestCancelledError() *types.NewAPIError {
	return types.NewOpenAIError(
		ErrInFlightRequestCancelled,
		types.ErrorCodeRequestCancelled,
		http.StatusServiceUnavailable,
		types.ErrOptionWithSkipRetry(),
	)
}

const inFlightRequestContextKeyName = "in_flight_request_context"

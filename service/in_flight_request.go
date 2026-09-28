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
	if ctxValue, ok := c.Get(virtualPoolAttemptContextKeyName); ok {
		if ctx, ok := ctxValue.(context.Context); ok && ctx != nil {
			return ctx
		}
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

// SetVirtualPoolAttemptContext installs a child context for one relay
// attempt. Replacing the context is safe because RelayRequestContext prefers
// it over the request and in-flight cancellation contexts.
func SetVirtualPoolAttemptContext(c *gin.Context, ctx context.Context) {
	if c == nil {
		return
	}
	c.Set(virtualPoolAttemptContextKeyName, ctx)
}

// ClearVirtualPoolAttemptContext removes the per-attempt context after the
// attempt has been released. The cancellation function remains owned by the
// scheduled candidate until ReleaseVirtualPoolAttempt is called.
func ClearVirtualPoolAttemptContext(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(virtualPoolAttemptContextKeyName, nil)
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
const virtualPoolAttemptContextKeyName = "virtual_pool_attempt_context"

package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installVirtualPoolStickyForTest(t *testing.T, setting operation_setting.VirtualPoolStickySetting) {
	t.Helper()
	retrySetting := operation_setting.GetModelRetryPolicySetting()
	original := retrySetting.VirtualPoolSticky
	retrySetting.VirtualPoolSticky = setting
	t.Cleanup(func() {
		retrySetting.VirtualPoolSticky = original
	})
}

func newVirtualPoolSessionContext(t *testing.T, headers map[string][]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = request
	common.SetContextKey(ctx, constant.ContextKeyUserId, 11)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 22)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	return ctx
}

func TestResolveVirtualPoolSessionScopesAndPrioritizesHeaders(t *testing.T) {
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:     true,
		SessionMode: operation_setting.VirtualPoolSessionModeThread,
	})

	ctx := newVirtualPoolSessionContext(t, map[string][]string{
		"X-NewAPI-Session-ID": {"explicit"},
		"thread-id":           {"thread"},
		"session-id":          {"root"},
	})
	session, err := ResolveVirtualPoolSession(ctx, "deepseek v4.1 flash")
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "x-newapi-session-id", session.Source)
	assert.NotContains(t, session.CacheKey, "explicit")
	assert.NotContains(t, session.CacheKey, "deepseek v4.1 flash")

	otherCtx := newVirtualPoolSessionContext(t, map[string][]string{"X-NewAPI-Session-ID": {"explicit"}})
	common.SetContextKey(otherCtx, constant.ContextKeyTokenId, 23)
	other, err := ResolveVirtualPoolSession(otherCtx, "deepseek v4.1 flash")
	require.NoError(t, err)
	require.NotNil(t, other)
	assert.NotEqual(t, session.CacheKey, other.CacheKey, "token isolation is part of the scope")
}

func TestResolveVirtualPoolSessionRejectsConflictingAliases(t *testing.T) {
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:     true,
		SessionMode: operation_setting.VirtualPoolSessionModeThread,
	})
	ctx := newVirtualPoolSessionContext(t, map[string][]string{
		"thread-id":  {"one"},
		"thread_id":  {"two"},
		"session-id": {"root"},
	})

	_, err := ResolveVirtualPoolSession(ctx, "deepseek v4.1 flash")
	require.Error(t, err)
	assert.ErrorIs(t, err, errVirtualPoolSessionConflict)
}

func TestResolveVirtualPoolSessionIgnoresMissingAndDisabled(t *testing.T) {
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{})
	ctx := newVirtualPoolSessionContext(t, map[string][]string{"thread-id": {"thread"}})
	session, err := ResolveVirtualPoolSession(ctx, "deepseek v4.1 flash")
	require.NoError(t, err)
	assert.Nil(t, session)

	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:     true,
		SessionMode: operation_setting.VirtualPoolSessionModeThread,
	})
	ctx = newVirtualPoolSessionContext(t, nil)
	session, err = ResolveVirtualPoolSession(ctx, "deepseek v4.1 flash")
	require.NoError(t, err)
	assert.Nil(t, session, "a request without an ID must not share a constant binding key")
}

func TestResolveVirtualPoolSessionIgnoresNonStringBodyFields(t *testing.T) {
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:             true,
		SessionMode:         operation_setting.VirtualPoolSessionModeThread,
		AllowPromptCacheKey: true,
	})
	ctx := newVirtualPoolSessionContext(t, nil)
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(
		`{"model":"deepseek v4.1 flash","prompt_cache_key":{"scope":"ignored"}}`,
	))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUserId, 11)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 22)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	session, err := ResolveVirtualPoolSession(ctx, "deepseek v4.1 flash")
	require.NoError(t, err)
	assert.Nil(t, session)
}

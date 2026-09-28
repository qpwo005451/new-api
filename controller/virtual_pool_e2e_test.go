package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestMain disables Redis once, before any controller test starts goroutines
// that read common.RedisEnabled. Per-test cleanup cannot safely restore that
// global while async quota-cache goroutines are still reading it.
func TestMain(m *testing.M) {
	common.RedisEnabled = false
	os.Exit(m.Run())
}

// virtualPoolE2EUpstream is a fake provider that records how many times it was
// asked to execute a request. The partial-stream mode closes the connection
// after a client-visible chunk without ever sending a finish reason.
type virtualPoolE2EUpstream struct {
	server *httptest.Server
	calls  atomic.Int64
}

func newVirtualPoolE2EUpstream(t *testing.T, partialStream bool) *virtualPoolE2EUpstream {
	t.Helper()
	upstream := &virtualPoolE2EUpstream{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstream.calls.Add(1)
		if partialStream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"upstream\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			// The stream handler holds back the most recent event until the next
			// one arrives, so emit a second chunk to force the first one out.
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chunk-2\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"upstream\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"tail\"},\"finish_reason\":null}]}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chatcmpl-e2e","object":"chat.completion","created":1,"model":"upstream","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

type virtualPoolE2EFixture struct {
	router       *gin.Engine
	virtualModel string
	upstreamA    *virtualPoolE2EUpstream
	upstreamB    *virtualPoolE2EUpstream
}

// setupVirtualPoolE2EFixture wires the real Distribute middleware and Relay
// controller against a SQLite fixture and two fake upstreams.
func setupVirtualPoolE2EFixture(
	t *testing.T,
	stickyEnabled bool,
	rotation string,
	partialStream bool,
) *virtualPoolE2EFixture {
	t.Helper()

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousRetryTimes := common.RetryTimes
	previousLogConsumeEnabled := common.LogConsumeEnabled
	previousInFlightUsageLogEnabled := common.InFlightUsageLogEnabled
	previousSelfUseMode := operation_setting.SelfUseModeEnabled
	previousStreamingTimeout := constant.StreamingTimeout

	common.MemoryCacheEnabled = true
	common.RetryTimes = 2
	common.LogConsumeEnabled = false
	common.InFlightUsageLogEnabled = false
	constant.StreamingTimeout = 30
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	operation_setting.SelfUseModeEnabled = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Channel{},
		&model.Ability{},
		&model.ChannelBalanceProtection{},
	))
	model.DB, model.LOG_DB = db, db

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.RetryTimes = previousRetryTimes
		common.LogConsumeEnabled = previousLogConsumeEnabled
		common.InFlightUsageLogEnabled = previousInFlightUsageLogEnabled
		constant.StreamingTimeout = previousStreamingTimeout
		operation_setting.SelfUseModeEnabled = previousSelfUseMode
		common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
		if previousMemoryCacheEnabled && previousDB != nil &&
			previousDB.Migrator().HasTable(&model.Channel{}) && previousDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.User{
		Id:       9101,
		Username: "virtual-pool-e2e",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    1 << 30,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:             9201,
		UserId:         9101,
		Key:            "virtual-pool-e2e-token",
		Name:           "virtual-pool-e2e",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    1 << 30,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	upstreamA := newVirtualPoolE2EUpstream(t, partialStream)
	upstreamB := newVirtualPoolE2EUpstream(t, partialStream)

	priority := int64(0)
	weight := uint(1)
	createChannel := func(id int, name, upstreamModel, baseURL, key string) {
		require.NoError(t, db.Create(&model.Channel{
			Id:       id,
			Type:     constant.ChannelTypeOpenAI,
			Key:      key,
			Status:   common.ChannelStatusEnabled,
			Name:     name,
			Weight:   &weight,
			Models:   upstreamModel,
			Group:    "default",
			Priority: &priority,
			BaseURL:  &baseURL,
		}).Error)
		require.NoError(t, db.Create(&model.Ability{
			Group:     "default",
			Model:     upstreamModel,
			ChannelId: id,
			Enabled:   true,
			Priority:  &priority,
			Weight:    weight,
		}).Error)
	}
	createChannel(9401, "virtual-pool-e2e-a", "vpool-upstream-a", upstreamA.server.URL, "key-a")
	createChannel(9402, "virtual-pool-e2e-b", "vpool-upstream-b", upstreamB.server.URL, "key-b")

	virtualModel := "e2e-virtual-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	originalSticky := retrySetting.VirtualPoolSticky
	routes := make(map[string]operation_setting.VirtualModelRoute, len(originalRoutes)+1)
	for name, route := range originalRoutes {
		routes[name] = route
	}
	routes[virtualModel] = operation_setting.VirtualModelRoute{
		Rotation:    rotation,
		MaxAttempts: 2,
		Targets: []operation_setting.VirtualModelRouteTarget{
			{Model: "vpool-upstream-a", Capacity: 1, Weight: 1},
			{Model: "vpool-upstream-b", Capacity: 1, Weight: 1},
		},
	}
	retrySetting.VirtualModelRoutes = routes
	retrySetting.VirtualPoolSticky = operation_setting.VirtualPoolStickySetting{
		Enabled:        stickyEnabled,
		SessionMode:    operation_setting.VirtualPoolSessionModeThread,
		MultiKeyPolicy: operation_setting.VirtualPoolMultiKeyPolicyBindIndex,
		BindingMode:    operation_setting.VirtualPoolBindingModeMemory,
	}
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
		retrySetting.VirtualPoolSticky = originalSticky
	})
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestId())
	router.Use(middleware.BodyStorageCleanup())
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, 9101)
		common.SetContextKey(c, constant.ContextKeyTokenId, 9201)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "virtual-pool-e2e-token")
		common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		c.Next()
	})
	router.Use(middleware.Distribute())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		Relay(c, types.RelayFormatOpenAI)
	})

	return &virtualPoolE2EFixture{
		router:       router,
		virtualModel: virtualModel,
		upstreamA:    upstreamA,
		upstreamB:    upstreamB,
	}
}

func (fixture *virtualPoolE2EFixture) post(t *testing.T, sessionID string, stream bool) *httptest.ResponseRecorder {
	t.Helper()
	streamField := ""
	if stream {
		streamField = `"stream":true,`
	}
	body := fmt.Sprintf(
		`{"model":%q,%s"messages":[{"role":"user","content":"ping"}]}`,
		fixture.virtualModel,
		streamField,
	)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		request.Header.Set("X-NewAPI-Session-ID", sessionID)
	}
	recorder := httptest.NewRecorder()
	fixture.router.ServeHTTP(recorder, request)
	return recorder
}

func TestVirtualPoolE2EStickySessionReusesBoundCandidate(t *testing.T) {
	fixture := setupVirtualPoolE2EFixture(t, true, operation_setting.VirtualModelRouteRotationOrdered, false)

	first := fixture.post(t, "sticky-session", false)
	require.Equal(t, http.StatusOK, first.Code)
	second := fixture.post(t, "sticky-session", false)
	require.Equal(t, http.StatusOK, second.Code)

	totalCalls := fixture.upstreamA.calls.Load() + fixture.upstreamB.calls.Load()
	require.Equal(t, int64(2), totalCalls)
	assert.True(
		t,
		fixture.upstreamA.calls.Load() == 2 || fixture.upstreamB.calls.Load() == 2,
		"the second request must reuse the confirmed candidate: a=%d b=%d",
		fixture.upstreamA.calls.Load(),
		fixture.upstreamB.calls.Load(),
	)
}

func TestVirtualPoolE2EPreparesCandidatesOncePerRequest(t *testing.T) {
	fixture := setupVirtualPoolE2EFixture(t, false, operation_setting.VirtualModelRouteRotationRoundRobin, false)

	first := fixture.post(t, "", false)
	require.Equal(t, http.StatusOK, first.Code)
	second := fixture.post(t, "", false)
	require.Equal(t, http.StatusOK, second.Code)

	assert.Equal(t, int64(1), fixture.upstreamA.calls.Load(), "rotation advanced exactly once for the first request")
	assert.Equal(t, int64(1), fixture.upstreamB.calls.Load(), "a second preparation in the controller would advance rotation twice and reuse the first candidate")
}

func TestVirtualPoolE2ECommittedPartialStreamIsNotReplayed(t *testing.T) {
	fixture := setupVirtualPoolE2EFixture(t, true, operation_setting.VirtualModelRouteRotationOrdered, true)

	recorder := fixture.post(t, "partial-stream-session", true)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "partial")
	assert.Contains(t, recorder.Body.String(), "upstream_stream_terminated")
	assert.Equal(t, int64(1), fixture.upstreamA.calls.Load(), "the committed attempt ran on the first candidate exactly once")
	assert.Equal(t, int64(0), fixture.upstreamB.calls.Load(), "a committed partial stream must never be replayed on another upstream")
}

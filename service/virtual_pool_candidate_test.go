package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedVirtualPoolRouteBindsMultiKeyAccount(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-multikey"
	createMultiModelSelectChannel(t, db, 2601, "default", "multikey-model")
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2601).Updates(map[string]any{
		"key":    "key-a\nkey-b\nkey-c",
		"weight": weight,
	}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2601).Update("priority", priority).Error)
	model.InitChannelCache()
	channel, err := model.CacheGetChannel(2601)
	require.NoError(t, err)
	channel.ChannelInfo = model.ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         len(channel.GetKeys()),
		MultiKeyStatusList:   map[int]int{0: common.ChannelStatusEnabled, 1: common.ChannelStatusEnabled, 2: common.ChannelStatusEnabled},
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 0,
	}
	model.CacheUpdateChannel(channel)
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Targets: []operation_setting.VirtualModelRouteTarget{{Model: "multikey-model"}},
	})
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:        true,
		MultiKeyPolicy: operation_setting.VirtualPoolMultiKeyPolicyBindIndex,
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserId, 1)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 2)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	session := &VirtualPoolSession{SessionDigest: common.Sha1([]byte("session"))}

	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	}
	param.SetVirtualPoolSession(session)
	selected, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	prepared := param.PreparedVirtualPoolRoute()
	require.NotNil(t, prepared)
	require.Len(t, prepared.Candidates, 1)
	assert.Equal(t, 2601, prepared.Candidates[0].Channel.Id)
	assert.Contains(t, prepared.Candidates[0].AccountIdentity, fmt.Sprintf("%d:", 2601))
	assert.GreaterOrEqual(t, prepared.Candidates[0].KeyIndex, 0)
	assert.Less(t, prepared.Candidates[0].KeyIndex, 3)
	assert.NotContains(t, prepared.Candidates[0].AccountIdentity, "key-a")

	reused := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	}
	reused.AdoptPreparedVirtualPoolRoute(prepared)
	candidate, ok := reused.NextPreparedVirtualPoolCandidate()
	require.True(t, ok)
	assert.Equal(t, prepared.Candidates[0].AccountIdentity, candidate.AccountIdentity)
	_, ok = reused.NextPreparedVirtualPoolCandidate()
	assert.False(t, ok, "the same execution identity must not be attempted twice")
}

func TestPreparedVirtualPoolRouteDropsDisabledBoundKey(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-multikey-disabled"
	createMultiModelSelectChannel(t, db, 2602, "default", "multikey-model")
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2602).Updates(map[string]any{
		"key":    "key-a\nkey-b",
		"weight": weight,
	}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2602).Update("priority", priority).Error)
	model.InitChannelCache()
	channel, err := model.CacheGetChannel(2602)
	require.NoError(t, err)
	channel.ChannelInfo = model.ChannelInfo{
		IsMultiKey:         true,
		MultiKeySize:       len(channel.GetKeys()),
		MultiKeyStatusList: map[int]int{0: common.ChannelStatusEnabled, 1: common.ChannelStatusEnabled},
		MultiKeyMode:       constant.MultiKeyModePolling,
	}
	model.CacheUpdateChannel(channel)
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Targets: []operation_setting.VirtualModelRouteTarget{{Model: "multikey-model"}},
	})
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:        true,
		MultiKeyPolicy: operation_setting.VirtualPoolMultiKeyPolicyBindIndex,
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	session := &VirtualPoolSession{SessionDigest: common.Sha1([]byte("disabled-session"))}
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	}
	param.SetVirtualPoolSession(session)
	selected, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	boundIndex := param.PreparedVirtualPoolRoute().Candidates[0].KeyIndex

	channel.ChannelInfo.MultiKeyStatusList = map[int]int{boundIndex: common.ChannelStatusManuallyDisabled}
	model.CacheUpdateChannel(channel)
	reused := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	}
	reused.SetVirtualPoolSession(session)
	selected, _, err = CacheGetRandomSatisfiedChannel(reused)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.NotEqual(t, boundIndex, reused.PreparedVirtualPoolRoute().Candidates[0].KeyIndex,
		"a disabled bound key must be replaced before the request is sent")
}

func TestVirtualPoolCandidateDeduplicatesByExecutionIdentity(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-dedup"
	createMultiModelSelectChannel(t, db, 2611, "default", "same-model")
	model.InitChannelCache()
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Targets: []operation_setting.VirtualModelRouteTarget{{Model: "same-model"}},
		Sources: []operation_setting.VirtualModelRouteSource{{ChannelId: 2611}},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	param := newVirtualRouteSourceGroup(ctx, modelName)
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	prepared := param.PreparedVirtualPoolRoute()
	require.NotNil(t, prepared)
	assert.Len(t, prepared.Candidates, 1, "target and source expansions of the same account/model collapse")
}

func TestVirtualPoolCandidateUsesFinalMappedModelForIdentity(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-mapped"
	createMultiModelSelectChannel(t, db, 2612, "default", "target-model")
	mapping := `{"target-model":"upstream-model"}`
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2612).Update("model_mapping", mapping).Error)
	model.InitChannelCache()
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Targets: []operation_setting.VirtualModelRouteTarget{{Model: "target-model"}},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	param := newVirtualRouteSourceGroup(ctx, modelName)
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)

	candidate := param.PreparedVirtualPoolRoute().Candidates[0]
	assert.Equal(t, "upstream-model", candidate.FinalMappedModel)
	assert.Contains(t, candidate.AttemptKey(), "upstream-model")
	assert.NotContains(t, candidate.AttemptKey(), "target-model", "identity must use the post-mapping model")
}

func TestPreparedVirtualPoolCandidatePeekDoesNotConsumeFirstAttempt(t *testing.T) {
	param := &RetryParam{
		preparedRoute: &VirtualPoolPreparedRoute{
			Limit: 1,
			Candidates: []VirtualPoolCandidate{{
				AccountIdentity:  "account-a",
				FinalMappedModel: "model",
			}},
		},
	}

	peeked, ok := param.PeekPreparedVirtualPoolCandidate()
	require.True(t, ok)
	assert.Equal(t, "account-a", peeked.AccountIdentity)

	selected, ok := param.NextPreparedVirtualPoolCandidate()
	require.True(t, ok)
	assert.Equal(t, peeked.AccountIdentity, selected.AccountIdentity)
	_, ok = param.NextPreparedVirtualPoolCandidate()
	assert.False(t, ok, "the controller must consume the peeked candidate exactly once")
}

func TestRetryParamConsumesScheduledCandidateBeforeRotation(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{
		Ctx:          ctx,
		virtualReady: true,
		preparedRoute: &VirtualPoolPreparedRoute{
			Limit: 2,
			Candidates: []VirtualPoolCandidate{
				{Channel: &model.Channel{Id: 1, Name: "rotated"}, AccountIdentity: "channel:1", FinalMappedModel: "model"},
				{Channel: &model.Channel{Id: 2, Name: "scheduled"}, AccountIdentity: "channel:2", FinalMappedModel: "model"},
			},
		},
		virtualRoute: []virtualRouteCandidate{
			{channel: &model.Channel{Id: 1, Name: "rotated"}, accountIdentity: "channel:1", finalMappedModel: "model"},
			{channel: &model.Channel{Id: 2, Name: "scheduled"}, accountIdentity: "channel:2", finalMappedModel: "model"},
		},
		virtualLimit:  2,
		virtualOffset: 0,
		scheduled: &VirtualPoolScheduledCandidate{Candidate: VirtualPoolCandidate{
			Channel:          &model.Channel{Id: 2, Name: "scheduled"},
			UpstreamModel:    "scheduled-model",
			ReasoningEffort:  "high",
			Group:            "vip",
			AccountIdentity:  "channel:2",
			FinalMappedModel: "model",
		}},
	}

	channel, group, handled, err := param.getVirtualRouteChannel()
	require.NoError(t, err)
	require.True(t, handled)
	require.NotNil(t, channel)
	assert.Equal(t, 2, channel.Id, "the candidate holding the scheduled lease must be sent")
	assert.Equal(t, "vip", group)
	assert.Equal(t, "scheduled-model", common.GetContextKeyString(ctx, constant.ContextKeyVirtualUpstreamModel))
	assert.Equal(t, "high", common.GetContextKeyString(ctx, constant.ContextKeyVirtualReasoningEffort))
	assert.True(t, param.scheduledTaken)
}

func TestPreparedVirtualPoolRouteIsNotRepreparedOrRerotated(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-prepared-once"
	for _, id := range []int{2631, 2632, 2633} {
		createMultiModelSelectChannel(t, db, id, "default", "prepared-model")
	}
	model.InitChannelCache()
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Rotation: operation_setting.VirtualModelRouteRotationRoundRobin,
		Targets:  []operation_setting.VirtualModelRouteTarget{{Model: "prepared-model"}},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	param := newVirtualRouteSourceGroup(ctx, modelName)
	_, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	prepared := param.PreparedVirtualPoolRoute()
	require.NotNil(t, prepared)
	require.Len(t, prepared.Candidates, 3)

	reused := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	}
	reused.AdoptPreparedVirtualPoolRoute(prepared)
	assert.Same(t, prepared, reused.PreparedVirtualPoolRoute(), "the controller must reuse the middleware-owned candidate list")

	first, ok := reused.PeekPreparedVirtualPoolCandidate()
	require.True(t, ok)
	selected, ok := reused.NextPreparedVirtualPoolCandidate()
	require.True(t, ok)
	assert.Equal(t, first.AccountIdentity, selected.AccountIdentity)
	assert.Equal(t, prepared.Offset, reused.virtualOffset, "adoption must not advance rotation")
}

func TestCleanupPreparedVirtualPoolAttemptReleasesUnstartedLease(t *testing.T) {
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:        true,
		BindingMode:    operation_setting.VirtualPoolBindingModeMemory,
		MultiKeyPolicy: operation_setting.VirtualPoolMultiKeyPolicyBindIndex,
	})
	bindings, capacity := getVirtualPoolMemoryStores()
	scheduler := NewVirtualPoolScheduler(bindings, capacity)
	now := time.Now()
	candidate := VirtualPoolCandidate{
		AccountIdentity:  "cleanup-account",
		FinalMappedModel: "cleanup-model",
		Capacity:         1,
	}
	scheduled, err := scheduler.Select(context.Background(), "cleanup-session", "cleanup-owner", []VirtualPoolCandidate{candidate}, now)
	require.NoError(t, err)
	require.NotNil(t, scheduled)

	param := &RetryParam{
		preparedRoute: &VirtualPoolPreparedRoute{
			Scheduled: scheduled,
			scheduler: scheduler,
		},
	}
	param.CleanupPreparedVirtualPoolAttempt(nil)
	assert.Nil(t, param.preparedRoute.Scheduled)
	assert.Nil(t, param.preparedRoute.scheduler)

	active, err := capacity.Active(context.Background(), scheduled.CapacityKey, time.Now().Add(time.Second))
	require.NoError(t, err)
	require.Zero(t, active, "cleanup must release the scheduled capacity owner")
	_, acquired, err := capacity.Acquire(context.Background(), scheduled.CapacityKey, "other-owner", 1, time.Now().Add(time.Second), time.Minute)
	require.NoError(t, err)
	assert.True(t, acquired, "middleware cleanup must free an unstarted capacity lease")
}

package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModelHealthCooldowns(t *testing.T) {
	setting := operation_setting.GetModelHealthPolicySetting()
	orig := *setting
	setting.Enabled = true
	setting.Rules = []operation_setting.ModelHealthPolicyRule{{
		Name:             "snapshot rule",
		Enabled:          true,
		Models:           []string{"snap-model"},
		FailureThreshold: 2,
		CooldownSeconds:  300,
		StatusCodes:      []int{503},
	}}
	t.Cleanup(func() { *setting = orig })

	RecordModelHealthFailure(nil, "snap-model", "default", 2891, http.StatusServiceUnavailable)
	RecordModelHealthFailure(nil, "snap-model", "default", 2891, http.StatusServiceUnavailable)

	snapshot := ListModelHealthCooldowns()
	var entry *ModelHealthCooldownEntry
	for i := range snapshot.Entries {
		if snapshot.Entries[i].ModelName == "snap-model" && snapshot.Entries[i].ChannelID == 2891 {
			entry = &snapshot.Entries[i]
			break
		}
	}
	require.NotNil(t, entry, "cooling entry must appear in the snapshot")
	assert.True(t, entry.Cooling)
	assert.Equal(t, 2, entry.ConsecutiveErrs)
	assert.Greater(t, entry.CooldownUntil, time.Now().Unix())
}

func TestResetModelHealthState(t *testing.T) {
	t.Cleanup(func() { ResetModelHealthState() })
	ResetModelHealthState()

	setting := operation_setting.GetModelHealthPolicySetting()
	orig := *setting
	setting.Enabled = true
	setting.Rules = []operation_setting.ModelHealthPolicyRule{{
		Name:             "snapshot reset rule",
		Enabled:          true,
		Models:           []string{"snap-reset-model"},
		FailureThreshold: 1,
		CooldownSeconds:  300,
		StatusCodes:      []int{503},
	}}
	t.Cleanup(func() { *setting = orig })

	RecordModelHealthFailure(nil, "snap-reset-model", "default", 2892, http.StatusServiceUnavailable)
	require.Len(t, ListModelHealthCooldowns().Entries, 1)

	cleared := ResetModelHealthState()
	assert.Equal(t, 1, cleared)
	assert.Empty(t, ListModelHealthCooldowns().Entries)
}

func TestModelHealthBindingMode(t *testing.T) {
	assert.Equal(t, "memory", ModelHealthBindingMode())
}

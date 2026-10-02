package service

import (
	"sort"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedChannelAffinityBinding replaces the configured rules with one rule that
// names the rule, the model and the group in the cache key, then binds the
// given sessions to their channels. The affinity cache is a package singleton,
// so both the rules and the seeded entries are restored on cleanup.
func seedChannelAffinityBinding(t *testing.T, bindings map[string]int) {
	t.Helper()

	setting := operation_setting.GetChannelAffinitySetting()
	originalRules := setting.Rules
	t.Cleanup(func() { setting.Rules = originalRules })
	setting.Rules = []operation_setting.ChannelAffinityRule{{
		Name:              "routing stats bindings",
		IncludeRuleName:   true,
		IncludeModelName:  true,
		IncludeUsingGroup: true,
	}}

	cache := getChannelAffinityCache()
	suffixes := make([]string, 0, len(bindings))
	for suffix, channelID := range bindings {
		fullKey := channelAffinityCacheNamespace + ":" + suffix
		require.NoError(t, cache.SetWithTTL(fullKey, channelID, time.Minute))
		suffixes = append(suffixes, fullKey)
	}
	t.Cleanup(func() {
		_, err := cache.DeleteMany(suffixes)
		require.NoError(t, err)
	})
}

// TestListChannelAffinityBindingsReportsTheBoundChannel covers the live view the
// routing page shows: which channel each session is currently pinned to, with
// the session value reduced to its hint and fingerprint.
func TestListChannelAffinityBindingsReportsTheBoundChannel(t *testing.T) {
	seedChannelAffinityBinding(t, map[string]int{
		"routing stats bindings:deepseek-v4.1-flash:default:session-that-is-long": 9,
		"routing stats bindings:deepseek-v4.1-flash:default:short":                47,
		"routing stats bindings:glm-5.3-flash:default:session-that-is-long":       9,
	})

	bindings := ListChannelAffinityBindings(0)

	require.False(t, bindings.Truncated)
	assert.EqualValues(t, 0, bindings.Unknown)
	assert.Equal(t, []ChannelAffinityBinding{
		{
			RuleName:       "routing stats bindings",
			ModelName:      "deepseek-v4.1-flash",
			UsingGroup:     "default",
			KeyHint:        "short",
			KeyFingerprint: affinityFingerprint("short"),
			ChannelID:      47,
		},
		{
			RuleName:       "routing stats bindings",
			ModelName:      "deepseek-v4.1-flash",
			UsingGroup:     "default",
			KeyHint:        "sess...long",
			KeyFingerprint: affinityFingerprint("session-that-is-long"),
			ChannelID:      9,
		},
		{
			RuleName:       "routing stats bindings",
			ModelName:      "glm-5.3-flash",
			UsingGroup:     "default",
			KeyHint:        "sess...long",
			KeyFingerprint: affinityFingerprint("session-that-is-long"),
			ChannelID:      9,
		},
	}, bindings.Entries)
}

// TestListChannelAffinityBindingsCapsTheEntryList protects the admin page from
// an unbounded cache read.
func TestListChannelAffinityBindingsCapsTheEntryList(t *testing.T) {
	seedChannelAffinityBinding(t, map[string]int{
		"routing stats bindings:m1:default:a": 9,
		"routing stats bindings:m2:default:b": 9,
		"routing stats bindings:m3:default:c": 9,
	})

	bindings := ListChannelAffinityBindings(2)

	assert.True(t, bindings.Truncated)
	assert.Len(t, bindings.Entries, 2)
}

// TestListChannelAffinityBindingsCountsUnparsableKeys keeps keys written by an
// older rule set visible as unknown instead of reporting them as bindings.
func TestListChannelAffinityBindingsCountsUnparsableKeys(t *testing.T) {
	seedChannelAffinityBinding(t, map[string]int{
		"routing stats bindings:deepseek-v4.1-flash:default:session": 9,
		"removed rule:deepseek-v4.1-flash:default:session":           9,
	})

	bindings := ListChannelAffinityBindings(0)

	assert.Len(t, bindings.Entries, 1)
	assert.EqualValues(t, 1, bindings.Unknown)
}

func TestListChannelAffinityBindingsSortsByRuleModelAndSession(t *testing.T) {
	seedChannelAffinityBinding(t, map[string]int{
		"routing stats bindings:b-model:z-group:aaa": 9,
		"routing stats bindings:b-model:a-group:bbb": 9,
		"routing stats bindings:a-model:a-group:ccc": 9,
	})

	bindings := ListChannelAffinityBindings(0)

	keys := make([]string, 0, len(bindings.Entries))
	for _, entry := range bindings.Entries {
		keys = append(keys, entry.ModelName+"/"+entry.UsingGroup+"/"+entry.KeyHint)
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	assert.Equal(t, sorted, keys)
}

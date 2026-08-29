package operation_setting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeClientAliasesUpsertTrimsAndCopies(t *testing.T) {
	current := map[string]string{"old/ua": "Old Client"}

	next, err := NormalizeClientAliases(current, "  OpenAI/JS 6.47.0  ", "  Prime Agent  ")
	require.NoError(t, err)
	assert.Equal(t, "Prime Agent", next["OpenAI/JS 6.47.0"])
	assert.Equal(t, "Old Client", next["old/ua"])
	assert.NotContains(t, current, "OpenAI/JS 6.47.0", "input map must not be mutated")
}

func TestNormalizeClientAliasesEmptyNameRemovesEntry(t *testing.T) {
	current := map[string]string{"OpenAI/JS 6.47.0": "Prime Agent", "keep/ua": "Keep"}

	next, err := NormalizeClientAliases(current, "OpenAI/JS 6.47.0", "")
	require.NoError(t, err)
	assert.NotContains(t, next, "OpenAI/JS 6.47.0")
	assert.Len(t, next, 1)
	assert.Contains(t, current, "OpenAI/JS 6.47.0", "input map must not be mutated")
}

func TestNormalizeClientAliasesRejectsInvalidInput(t *testing.T) {
	_, err := NormalizeClientAliases(map[string]string{}, "   ", "Name")
	require.Error(t, err, "empty user agent must be rejected")

	_, err = NormalizeClientAliases(map[string]string{}, strings.Repeat("u", MaxClientAliasUserAgentLength+1), "Name")
	require.Error(t, err, "oversized user agent must be rejected")

	_, err = NormalizeClientAliases(map[string]string{}, "ua", strings.Repeat("名", MaxClientAliasNameLength+1))
	require.Error(t, err, "oversized name must be rejected")
}

func TestNormalizeClientAliasesRejectsFullTable(t *testing.T) {
	current := make(map[string]string, MaxClientAliasEntries)
	for i := 0; i < MaxClientAliasEntries; i++ {
		current[fmt.Sprintf("ua-%d", i)] = "Client"
	}

	_, err := NormalizeClientAliases(current, "new-ua", "New")
	require.Error(t, err, "inserting beyond the table cap must fail")

	next, err := NormalizeClientAliases(current, "ua-1", "Renamed")
	require.NoError(t, err, "renaming an existing entry must not consume a new slot")
	assert.Equal(t, "Renamed", next["ua-1"])
	assert.Len(t, next, MaxClientAliasEntries)
}

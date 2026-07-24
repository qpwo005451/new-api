package gemini

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelListIncludesGemini35FlashLite(t *testing.T) {
	assert.Contains(t, ModelList, "gemini-3.5-flash-lite")
}

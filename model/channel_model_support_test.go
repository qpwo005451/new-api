package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelSupportsModel(t *testing.T) {
	mapping := `{"gpt-5.6-sol":"upstream/gpt-5.6-sol"}`
	channel := Channel{
		Models:       "grok-4.5, gpt-5.6-terra",
		ModelMapping: &mapping,
	}

	assert.True(t, channel.SupportsModel("grok-4.5"))
	assert.True(t, channel.SupportsModel("gpt-5.6-terra"))
	assert.True(t, channel.SupportsModel("gpt-5.6-sol"))
	assert.False(t, channel.SupportsModel("gpt-5.6-luna"))
	assert.False(t, channel.SupportsModel(""))
}

func TestChannelSupportsModelRejectsInvalidMapping(t *testing.T) {
	mapping := `{"gpt-5.6-sol":`
	channel := Channel{ModelMapping: &mapping}

	assert.False(t, channel.SupportsModel("gpt-5.6-sol"))
}

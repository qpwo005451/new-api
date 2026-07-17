package controller

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBalanceExhaustionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "English balance", err: errors.New("insufficient balance"), want: true},
		{name: "English quota", err: errors.New("Quota Exhausted"), want: true},
		{name: "OpenAI quota code", err: errors.New("code=insufficient_quota"), want: true},
		{name: "Chinese balance", err: errors.New("上游余额不足"), want: true},
		{name: "unrelated rate limit", err: errors.New("rate limit exceeded"), want: false},
		{name: "unrelated auth", err: errors.New("invalid API key"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isBalanceExhaustionError(tt.err))
		})
	}
}

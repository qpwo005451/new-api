package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTransientRetryBackoff(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retryIndex int
		want       time.Duration
	}{
		{name: "first 429", statusCode: http.StatusTooManyRequests, retryIndex: 0, want: 300 * time.Millisecond},
		{name: "second 502", statusCode: http.StatusBadGateway, retryIndex: 1, want: 800 * time.Millisecond},
		{name: "third 503", statusCode: http.StatusServiceUnavailable, retryIndex: 2, want: 1500 * time.Millisecond},
		{name: "later transient retry stays bounded", statusCode: http.StatusServiceUnavailable, retryIndex: 8, want: 1500 * time.Millisecond},
		{name: "non transient status has no delay", statusCode: http.StatusInternalServerError, retryIndex: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, transientRetryBackoff(tt.statusCode, tt.retryIndex))
		})
	}
}

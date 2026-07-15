package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetWebSocketIdleTimeout(t *testing.T) {
	previous := GetPerformanceMonitorConfig()
	defer SetPerformanceMonitorConfig(previous)

	config := previous
	config.WebSocketIdleTimeoutMinutes = 10
	SetPerformanceMonitorConfig(config)
	assert.Equal(t, 10*time.Minute, GetWebSocketIdleTimeout())

	config.WebSocketIdleTimeoutMinutes = 0
	SetPerformanceMonitorConfig(config)
	assert.Zero(t, GetWebSocketIdleTimeout())
}

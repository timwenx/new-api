package performance_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConnectionLimits(t *testing.T) {
	assert.Equal(t, 10, GetPerformanceSetting().WebSocketIdleTimeoutMinutes)
	assert.Zero(t, GetPerformanceSetting().MaxIPsPerUser)
}

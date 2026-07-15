package performance_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultWebSocketIdleTimeoutMinutes(t *testing.T) {
	assert.Equal(t, 10, GetPerformanceSetting().WebSocketIdleTimeoutMinutes)
}

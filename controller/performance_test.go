package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/wsmanager"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisconnectWebSocketClosesSelectedConnection(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	defer func() {
		common.RedisEnabled = previousRedisEnabled
	}()

	closed := make(chan string, 1)
	unregister := wsmanager.RegisterWithInfo(987654, wsmanager.KindResponses, wsmanager.ConnectionInfo{
		UserID: 987654,
	}, func(reason string) {
		closed <- reason
	})
	defer unregister()

	var connection wsmanager.ConnectionInfo
	for _, item := range wsmanager.GetConnectionStats().Connections {
		if item.UserID == 987654 {
			connection = item
			break
		}
	}
	require.NotZero(t, connection.ConnectionID)
	require.NotEmpty(t, connection.NodeID)

	payload, err := common.Marshal(disconnectWebSocketRequest{
		ConnectionID: connection.ConnectionID,
		NodeID:       connection.NodeID,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/performance/websocket/disconnect", bytes.NewReader(payload))

	DisconnectWebSocket(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	select {
	case reason := <-closed:
		assert.Equal(t, service.AdministratorDisconnectedReason, reason)
	default:
		t.Fatal("selected WebSocket connection was not closed")
	}
	for _, item := range wsmanager.GetConnectionStats().Connections {
		assert.NotEqual(t, connection.ConnectionID, item.ConnectionID)
	}
}

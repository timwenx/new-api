package service

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestUsageLogPreservesWebSocketTransportDisplay(t *testing.T) {
	for _, tc := range []struct {
		name      string
		contextWS bool
		clientWS  *websocket.Conn
		wantWS    bool
	}{
		{name: "http"},
		{name: "official responses websocket", contextWS: true, wantWS: true},
		{name: "realtime websocket", clientWS: &websocket.Conn{}, wantWS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			ctx.Set("ws", tc.contextWS)
			start := time.Unix(1700000000, 0)
			info := &relaycommon.RelayInfo{
				StartTime:         start,
				FirstResponseTime: start.Add(1500 * time.Millisecond),
				ClientWs:          tc.clientWS,
				ChannelMeta:       &relaycommon.ChannelMeta{},
				StreamStatus:      relaycommon.NewStreamStatus(),
			}
			other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, 0, -1).Snapshot()
			assert.Equal(t, tc.wantWS, other["ws"] == true)
			assert.Equal(t, float64(1500), other["frt"])
			assert.Equal(t, "/v1/responses", other["request_path"])
		})
	}
}

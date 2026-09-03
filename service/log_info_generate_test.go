package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTextOtherInfoMarksWebSocketTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		clientWs         *websocket.Conn
		httpBridge       bool
		wantWSKey        bool
		wantWSToHTTPKey  bool
		upstreamFastMode bool
	}{
		{name: "http", wantWSKey: false, upstreamFastMode: false},
		{name: "websocket", clientWs: &websocket.Conn{}, wantWSKey: true, upstreamFastMode: true},
		{name: "responses websocket to http", clientWs: &websocket.Conn{}, httpBridge: true, wantWSKey: true, wantWSToHTTPKey: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
			startTime := time.Unix(1_700_000_000, 0)
			relayInfo := &relaycommon.RelayInfo{
				StartTime:           startTime,
				FirstResponseTime:   startTime.Add(1500 * time.Millisecond),
				ClientWs:            tt.clientWs,
				ResponsesHTTPBridge: tt.httpBridge,
				UpstreamFastMode:    tt.upstreamFastMode,
				ChannelMeta:         &relaycommon.ChannelMeta{},
			}

			other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)

			assert.Equal(t, float64(1500), other["frt"])
			assert.Equal(t, tt.upstreamFastMode, other["fast_mode"])
			ws, exists := other["ws"]
			assert.Equal(t, tt.wantWSKey, exists)
			if tt.wantWSKey {
				require.Equal(t, true, ws)
			}
			wsToHTTP, exists := other["ws_to_http"]
			assert.Equal(t, tt.wantWSToHTTPKey, exists)
			if tt.wantWSToHTTPKey {
				require.Equal(t, true, wsToHTTP)
			}
		})
	}
}

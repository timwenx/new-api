package common

import (
	"errors"
	"net"
	"time"

	appcommon "github.com/QuantumNous/new-api/common"

	"github.com/gorilla/websocket"
)

const WebSocketIdleCloseReason = "websocket idle timeout"

// RefreshClientWebSocketReadDeadline counts only data messages as activity.
// Gorilla handles Ping/Pong control frames inside ReadMessage, so heartbeats do
// not return to the caller and do not refresh this deadline.
func RefreshClientWebSocketReadDeadline(conn *websocket.Conn) error {
	if conn == nil {
		return errors.New("websocket connection is nil")
	}
	timeout := appcommon.GetWebSocketIdleTimeout()
	if timeout <= 0 {
		return conn.SetReadDeadline(time.Time{})
	}
	return conn.SetReadDeadline(time.Now().Add(timeout))
}

func IsWebSocketIdleTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

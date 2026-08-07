package common

import (
	"errors"
	"net"
	"time"

	appcommon "github.com/QuantumNous/new-api/common"

	"github.com/gorilla/websocket"
)

const (
	WebSocketInitialMessageCloseReason = "websocket initial message timeout"
	WebSocketIdleCloseReason           = "websocket idle timeout"
	webSocketInitialMessageTimeout     = 30 * time.Second
)

// SetClientWebSocketInitialReadDeadline requires the first application message
// within a fixed window, independent of the configured idle timeout.
func SetClientWebSocketInitialReadDeadline(conn *websocket.Conn) error {
	if conn == nil {
		return errors.New("websocket connection is nil")
	}
	return conn.SetReadDeadline(time.Now().Add(webSocketInitialMessageTimeout))
}

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

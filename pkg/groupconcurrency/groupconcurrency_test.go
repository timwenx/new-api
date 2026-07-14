package groupconcurrency

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	counters = make(map[key]*counter)
}

func TestAcquireSharesHTTPAndWebSocketLimitByUserAndGroup(t *testing.T) {
	resetForTest()

	releaseHTTP, allowed := Acquire(1, "alice", "default", TransportHTTP, 2)
	require.True(t, allowed)
	releaseWebSocket, allowed := Acquire(1, "alice", "default", TransportWebSocket, 2)
	require.True(t, allowed)

	_, allowed = Acquire(1, "alice", "default", TransportHTTP, 2)
	assert.False(t, allowed)

	usage := Snapshot()
	require.Len(t, usage, 1)
	assert.Equal(t, 1, usage[0].HTTPActive)
	assert.Equal(t, 1, usage[0].WebSocketActive)

	releaseWebSocket()
	releaseWebSocket()
	releaseAfterDisconnect, allowed := Acquire(1, "alice", "default", TransportHTTP, 2)
	require.True(t, allowed)

	releaseHTTP()
	releaseAfterDisconnect()
	assert.Empty(t, Snapshot())
}

func TestAcquireSeparatesUsersAndGroups(t *testing.T) {
	resetForTest()

	releaseDefault, allowed := Acquire(1, "alice", "default", TransportHTTP, 1)
	require.True(t, allowed)
	releaseVIP, allowed := Acquire(1, "alice", "vip", TransportHTTP, 1)
	require.True(t, allowed)
	releaseOtherUser, allowed := Acquire(2, "bob", "default", TransportHTTP, 1)
	require.True(t, allowed)

	_, allowed = Acquire(1, "alice", "default", TransportWebSocket, 1)
	assert.False(t, allowed)
	assert.Len(t, Snapshot(), 3)

	releaseDefault()
	releaseVIP()
	releaseOtherUser()
}

func TestAcquireTracksUnlimitedUsage(t *testing.T) {
	resetForTest()

	release, allowed := Acquire(1, "alice", "default", TransportWebSocket, 0)
	require.True(t, allowed)
	require.Len(t, Snapshot(), 1)

	release()
	assert.Empty(t, Snapshot())
}

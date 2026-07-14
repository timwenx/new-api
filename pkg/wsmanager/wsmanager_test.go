package wsmanager

import (
	"context"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetRegistryForTest() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[int]map[uint64]*entry{}
	nextID = 0
}

func TestCloseChannelClosesRegisteredConnectionsOnce(t *testing.T) {
	resetRegistryForTest()

	var mu sync.Mutex
	calls := 0
	Register(10, KindRealtime, func(reason string) {
		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, "test reason", reason, "close callback should receive the provided reason")
		calls++
	})
	Register(10, KindResponses, func(reason string) {
		mu.Lock()
		defer mu.Unlock()
		calls++
	})

	require.Equal(t, 2, CloseChannel(10, "test reason"), "CloseChannel should close every registered connection for the channel")
	require.Equal(t, 0, CloseChannel(10, "test reason"), "CloseChannel should not close already removed connections")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, calls, "all registered close callbacks should be called once")
}

func TestCloseChannelDoesNotCloseOtherChannels(t *testing.T) {
	resetRegistryForTest()

	calls := map[int]int{}
	Register(10, KindRealtime, func(reason string) {
		calls[10]++
	})
	Register(20, KindRealtime, func(reason string) {
		calls[20]++
	})

	require.Equal(t, 1, CloseChannel(10, "test"), "CloseChannel should only close the requested channel")
	assert.Equal(t, 1, calls[10], "requested channel callback should run")
	assert.Equal(t, 0, calls[20], "other channel callback should not run")
}

func TestUnregisterPreventsClose(t *testing.T) {
	resetRegistryForTest()

	calls := 0
	unregister := Register(10, KindRealtime, func(reason string) {
		calls++
	})
	unregister()

	require.Equal(t, 0, CloseChannel(10, "test"), "unregistered connections should not be closed")
	assert.Equal(t, 0, calls, "unregistered close callback should not run")
}

func TestRegisteredCloseIsIdempotent(t *testing.T) {
	resetRegistryForTest()

	calls := 0
	Register(10, KindRealtime, func(reason string) {
		calls++
	})

	mu.Lock()
	var registered *entry
	for _, e := range registry[10] {
		registered = e
	}
	mu.Unlock()
	require.NotNil(t, registered, "registered entry should exist")

	registered.close("test")
	registered.close("test")
	assert.Equal(t, 1, calls, "registered close callback should be idempotent")
}

func TestPublishCloseChannelsNoopsWhenRedisDisabled(t *testing.T) {
	resetRegistryForTest()

	oldEnabled := common.RedisEnabled
	oldRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	defer func() {
		common.RedisEnabled = oldEnabled
		common.RDB = oldRDB
	}()

	require.NoError(t, PublishCloseChannels(context.Background(), []int{10}, "test"), "publishing should no-op without Redis")
}

func TestGetConnectionStatsReturnsActiveUsersAndConnections(t *testing.T) {
	resetRegistryForTest()
	oldNodeName := common.NodeName
	common.NodeName = "test-node"
	defer func() {
		common.NodeName = oldNodeName
	}()

	unregister := RegisterWithInfo(20, KindResponses, ConnectionInfo{
		UserID:      1,
		Username:    "alice",
		TokenName:   "codex",
		Model:       "gpt-5.6-sol",
		Transport:   "ws",
		ConnectedAt: 200,
	}, func(string) {})
	defer unregister()
	unregisterSecond := RegisterWithInfo(10, KindResponses, ConnectionInfo{
		UserID:      1,
		Username:    "alice",
		TokenName:   "codex",
		Model:       "gpt-5.5",
		ConnectedAt: 100,
	}, func(string) {})
	defer unregisterSecond()
	unregisterThird := RegisterWithInfo(30, KindRealtime, ConnectionInfo{
		UserID:      2,
		Username:    "bob",
		TokenName:   "voice",
		Model:       "gpt-realtime",
		ConnectedAt: 150,
	}, func(string) {})
	defer unregisterThird()

	stats := GetConnectionStats()
	require.Len(t, stats.Connections, 3)
	assert.Equal(t, 3, stats.TotalConnections)
	assert.Equal(t, 2, stats.TotalUsers)
	assert.Equal(t, int64(100), stats.Connections[0].ConnectedAt)
	assert.Equal(t, "test-node", stats.Connections[0].NodeName)
	assert.Equal(t, KindResponses, stats.Connections[0].Kind)
	assert.Equal(t, "", stats.Connections[0].Transport)
	assert.Equal(t, 10, stats.Connections[0].ChannelID)
	assert.NotZero(t, stats.Connections[0].ConnectionID)
	assert.Equal(t, "ws", stats.Connections[2].Transport)

	unregister()
	assert.Equal(t, 2, GetConnectionStats().TotalConnections)
}

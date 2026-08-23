package useriplimit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	activeRequests = make(map[int]map[string]int)
}

func TestAcquireCountsDistinctIPsAndReleasesAfterLastRequest(t *testing.T) {
	resetForTest()

	releaseFirst, allowed := Acquire(1, "203.0.113.1", 2)
	require.True(t, allowed)
	releaseSecond, allowed := Acquire(1, "203.0.113.1", 2)
	require.True(t, allowed)
	releaseOtherIP, allowed := Acquire(1, "203.0.113.2", 2)
	require.True(t, allowed)

	_, allowed = Acquire(1, "203.0.113.3", 2)
	assert.False(t, allowed)

	releaseFirst()
	_, allowed = Acquire(1, "203.0.113.3", 2)
	assert.False(t, allowed)

	releaseSecond()
	releaseThirdIP, allowed := Acquire(1, "203.0.113.3", 2)
	require.True(t, allowed)
	releaseOtherIP()
	releaseThirdIP()
}

func TestAcquireSeparatesUsersAndTracksUnlimitedRequests(t *testing.T) {
	resetForTest()

	releaseFirst, allowed := Acquire(1, "203.0.113.1", 0)
	require.True(t, allowed)
	releaseSecond, allowed := Acquire(1, "203.0.113.2", 0)
	require.True(t, allowed)
	releaseOtherUser, allowed := Acquire(2, "203.0.113.3", 1)
	require.True(t, allowed)

	assert.Len(t, activeRequests[1], 2)
	assert.Len(t, activeRequests[2], 1)

	releaseFirst()
	releaseSecond()
	releaseOtherUser()
	assert.Empty(t, activeRequests)
}

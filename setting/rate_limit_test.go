package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupRateLimitSupportsLegacyAndConcurrencyValues(t *testing.T) {
	oldDefault := GetModelRequestConcurrencyLimit()
	oldJSON := ModelRequestRateLimitGroup2JSONString()
	defer func() {
		SetModelRequestConcurrencyLimit(oldDefault)
		require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(oldJSON))
	}()

	SetModelRequestConcurrencyLimit(3)
	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(`{"legacy":[10,5],"vip":[20,10,8],"unlimited":[0,100,0]}`))

	total, success, found := GetGroupRateLimit("legacy")
	assert.True(t, found)
	assert.Equal(t, 10, total)
	assert.Equal(t, 5, success)
	assert.Equal(t, 3, GetGroupConcurrencyLimit("legacy"))
	assert.Equal(t, 8, GetGroupConcurrencyLimit("vip"))
	assert.Equal(t, 0, GetGroupConcurrencyLimit("unlimited"))
	assert.Equal(t, 3, GetGroupConcurrencyLimit("missing"))
}

func TestCheckModelRequestRateLimitGroupRejectsInvalidConcurrency(t *testing.T) {
	assert.Error(t, CheckModelRequestRateLimitGroup(`{"default":[10,5,-1]}`))
	assert.Error(t, CheckModelRequestRateLimitGroup(`{"default":[10]}`))
	assert.NoError(t, CheckModelRequestRateLimitGroup(`{"default":[10,5,3]}`))
}

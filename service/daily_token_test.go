package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dailyTokenTestContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func dailyTokenUsageForServiceTest(t *testing.T, userId int, usageDate string) model.UserDailyTokenUsage {
	t.Helper()
	var usage model.UserDailyTokenUsage
	require.NoError(t, model.DB.Where("user_id = ? AND usage_date = ?", userId, usageDate).First(&usage).Error)
	return usage
}

func TestPreConsumeDailyTokensSettlesActualInputAndOutputUsage(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:          301,
		DailyTokenLimit: 1_000,
		StartTime:       time.Date(2026, time.August, 7, 23, 59, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 100, 400))
	assert.EqualValues(t, 500, dailyTokenUsageForServiceTest(t, 301, "2026-08-07").UsedTokens)

	SettleDailyTokens(dailyTokenTestContext(), info, 300)
	assert.EqualValues(t, 300, dailyTokenUsageForServiceTest(t, 301, "2026-08-07").UsedTokens)

	SettleDailyTokens(dailyTokenTestContext(), info, 900)
	assert.EqualValues(t, 300, dailyTokenUsageForServiceTest(t, 301, "2026-08-07").UsedTokens)
}

func TestRefundDailyTokensReleasesReservation(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:          302,
		DailyTokenLimit: 1_000,
		StartTime:       time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 100, 400))

	RefundDailyTokens(dailyTokenTestContext(), info)
	RefundDailyTokens(dailyTokenTestContext(), info)
	assert.Zero(t, dailyTokenUsageForServiceTest(t, 302, "2026-08-07").UsedTokens)
}

func TestDailyTokenSettlementAddsUsageBeyondReservation(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:          304,
		DailyTokenLimit: 1_000,
		StartTime:       time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 100, 400))
	SettleDailyTokens(dailyTokenTestContext(), info, 800)
	assert.EqualValues(t, 800, dailyTokenUsageForServiceTest(t, 304, "2026-08-07").UsedTokens)

	nextInfo := &relaycommon.RelayInfo{
		UserId:          304,
		DailyTokenLimit: 1_000,
		StartTime:       info.StartTime,
	}
	apiErr := PreConsumeDailyTokens(nextInfo, 100, 200)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeDailyTokenLimitExceeded, apiErr.GetErrorCode())
}

func TestPreConsumeDailyTokensReturnsRateLimitError(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:          303,
		DailyTokenLimit: 499,
		StartTime:       time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	apiErr := PreConsumeDailyTokens(info, 100, 400)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeDailyTokenLimitExceeded, apiErr.GetErrorCode())
	assert.Equal(t, 429, apiErr.StatusCode)
	assert.Nil(t, info.DailyTokens)
}

func TestDailyTokenReservationUsesFallbackWhenMaxOutputIsAbsent(t *testing.T) {
	oldPreConsumedQuota := common.PreConsumedQuota
	common.PreConsumedQuota = 500
	t.Cleanup(func() {
		common.PreConsumedQuota = oldPreConsumedQuota
	})

	assert.EqualValues(t, 500, dailyTokenReservationTokens(100, 0))
	assert.EqualValues(t, 700, dailyTokenReservationTokens(700, 0))
	assert.EqualValues(t, 900, dailyTokenReservationTokens(700, 200))
}

func TestEstimateRequestTokenStillCountsForDailyLimitWhenGlobalCountingIsDisabled(t *testing.T) {
	oldCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() {
		constant.CountToken = oldCountToken
	})

	tokens, err := EstimateRequestToken(
		dailyTokenTestContext(),
		&types.TokenCountMeta{TokenType: types.TokenTypeTextNumber, CombineText: "每日限额"},
		&relaycommon.RelayInfo{DailyTokenLimit: 1_000},
	)
	require.NoError(t, err)
	assert.Equal(t, 4, tokens)
}

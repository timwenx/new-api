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

func weeklyTokenUsageForServiceTest(t *testing.T, userId int, weekStart string) model.UserWeeklyTokenUsage {
	t.Helper()
	var usage model.UserWeeklyTokenUsage
	require.NoError(t, model.DB.Where("user_id = ? AND week_start = ?", userId, weekStart).First(&usage).Error)
	return usage
}

func modelWeeklyTokenUsageForServiceTest(t *testing.T, userId int, modelName string, weekStart string) model.UserModelWeeklyTokenUsage {
	t.Helper()
	var usage model.UserModelWeeklyTokenUsage
	require.NoError(t, model.DB.Where("user_id = ? AND model_name = ? AND week_start = ?", userId, modelName, weekStart).First(&usage).Error)
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

func TestPreConsumeWeeklyTokensSupportsBigIntLimitAndSettlesActualUsage(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:           305,
		WeeklyTokenLimit: 3_000_000_000,
		StartTime:        time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 100, 400))
	assert.EqualValues(t, 500, weeklyTokenUsageForServiceTest(t, 305, "2026-08-03").UsedTokens)

	SettleDailyTokens(dailyTokenTestContext(), info, 300)
	assert.EqualValues(t, 300, weeklyTokenUsageForServiceTest(t, 305, "2026-08-03").UsedTokens)
}

func TestPreConsumeWeeklyTokensReturnsRateLimitError(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:           306,
		WeeklyTokenLimit: 499,
		StartTime:        time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	apiErr := PreConsumeDailyTokens(info, 100, 400)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeWeeklyTokenLimitExceeded, apiErr.GetErrorCode())
	assert.Equal(t, 429, apiErr.StatusCode)
	assert.Nil(t, info.DailyTokens)
}

func TestPreConsumeModelWeeklyTokensKeepsDailyAndExcludesGeneralWeekly(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:                307,
		OriginModelName:       "gpt-special",
		DailyTokenLimit:       1_000,
		WeeklyTokenLimit:      200,
		ModelWeeklyTokenLimit: 3_000_000_000,
		StartTime:             time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 100, 400))
	assert.EqualValues(t, 500, dailyTokenUsageForServiceTest(t, 307, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 500, modelWeeklyTokenUsageForServiceTest(t, 307, "gpt-special", "2026-08-03").UsedTokens)

	var generalWeeklyRows int64
	require.NoError(t, model.DB.Model(&model.UserWeeklyTokenUsage{}).Where("user_id = ?", 307).Count(&generalWeeklyRows).Error)
	assert.Zero(t, generalWeeklyRows)

	SettleDailyTokens(dailyTokenTestContext(), info, 300)
	assert.EqualValues(t, 300, dailyTokenUsageForServiceTest(t, 307, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 300, modelWeeklyTokenUsageForServiceTest(t, 307, "gpt-special", "2026-08-03").UsedTokens)
}

func TestPreConsumeModelWeeklyTokensReturnsIndependentLimitError(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:                308,
		OriginModelName:       "gpt-special",
		DailyTokenLimit:       1_000,
		WeeklyTokenLimit:      1_000,
		ModelWeeklyTokenLimit: 499,
		StartTime:             time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	apiErr := PreConsumeDailyTokens(info, 100, 400)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeModelWeeklyTokenLimitExceeded, apiErr.GetErrorCode())
	assert.Equal(t, 429, apiErr.StatusCode)
	assert.Nil(t, info.DailyTokens)

	var dailyRows int64
	require.NoError(t, model.DB.Model(&model.UserDailyTokenUsage{}).Where("user_id = ?", 308).Count(&dailyRows).Error)
	assert.Zero(t, dailyRows)
	var generalWeeklyRows int64
	require.NoError(t, model.DB.Model(&model.UserWeeklyTokenUsage{}).Where("user_id = ?", 308).Count(&generalWeeklyRows).Error)
	assert.Zero(t, generalWeeklyRows)
}

func TestModelTokenMultiplierAppliesToDailyAndWeeklyLimits(t *testing.T) {
	truncate(t)

	requestTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local)
	info := &relaycommon.RelayInfo{
		UserId:           309,
		DailyTokenLimit:  10_000,
		WeeklyTokenLimit: 10_000,
		TokenMultiplier:  2,
		StartTime:        requestTime,
	}
	require.Nil(t, PreConsumeDailyTokens(info, 2_500, 2_500))
	assert.EqualValues(t, 10_000, dailyTokenUsageForServiceTest(t, 309, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 10_000, weeklyTokenUsageForServiceTest(t, 309, "2026-08-03").UsedTokens)

	SettleDailyTokens(dailyTokenTestContext(), info, 5_000)
	assert.EqualValues(t, 10_000, dailyTokenUsageForServiceTest(t, 309, "2026-08-07").UsedTokens)

	nextInfo := &relaycommon.RelayInfo{
		UserId:           309,
		DailyTokenLimit:  10_000,
		WeeklyTokenLimit: 10_000,
		TokenMultiplier:  2,
		StartTime:        requestTime,
	}
	apiErr := PreConsumeDailyTokens(nextInfo, 1, 1)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeDailyTokenLimitExceeded, apiErr.GetErrorCode())
}

func TestFastModeAppliesAfterModelMultiplierBeforeUpstream(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:           312,
		DailyTokenLimit:  15_000,
		WeeklyTokenLimit: 15_000,
		TokenMultiplier:  2,
		StartTime:        time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 2_500, 2_500))
	assert.EqualValues(t, 10_000, dailyTokenUsageForServiceTest(t, 312, "2026-08-07").UsedTokens)

	require.Nil(t, info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"fast"}`)))
	assert.EqualValues(t, 15_000, dailyTokenUsageForServiceTest(t, 312, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 15_000, weeklyTokenUsageForServiceTest(t, 312, "2026-08-03").UsedTokens)
	assert.Equal(t, 3.0, info.EffectiveTokenMultiplier())

	require.Nil(t, info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"flex"}`)))
	assert.EqualValues(t, 10_000, dailyTokenUsageForServiceTest(t, 312, "2026-08-07").UsedTokens)
	require.Nil(t, info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"fast"}`)))

	SettleDailyTokens(dailyTokenTestContext(), info, 5_000)
	assert.EqualValues(t, 15_000, dailyTokenUsageForServiceTest(t, 312, "2026-08-07").UsedTokens)
}

func TestFastModeExtraReservationHonorsTokenLimit(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:           313,
		DailyTokenLimit:  14_999,
		WeeklyTokenLimit: 14_999,
		TokenMultiplier:  2,
		StartTime:        time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 2_500, 2_500))

	apiErr := info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"fast"}`))
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeDailyTokenLimitExceeded, apiErr.GetErrorCode())
	assert.False(t, info.UpstreamFastMode)
	assert.EqualValues(t, 10_000, dailyTokenUsageForServiceTest(t, 313, "2026-08-07").UsedTokens)

	RefundDailyTokens(dailyTokenTestContext(), info)
	assert.Zero(t, dailyTokenUsageForServiceTest(t, 313, "2026-08-07").UsedTokens)
}

func TestFractionalModelTokenMultiplierReducesCountedUsage(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:           311,
		DailyTokenLimit:  3_750,
		WeeklyTokenLimit: 3_750,
		TokenMultiplier:  0.5,
		StartTime:        time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 2_500, 2_500))
	assert.EqualValues(t, 2_500, dailyTokenUsageForServiceTest(t, 311, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 2_500, weeklyTokenUsageForServiceTest(t, 311, "2026-08-03").UsedTokens)

	require.Nil(t, info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"priority"}`)))
	assert.EqualValues(t, 3_750, dailyTokenUsageForServiceTest(t, 311, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 3_750, weeklyTokenUsageForServiceTest(t, 311, "2026-08-03").UsedTokens)
}

func TestModelTokenMultiplierAppliesToIndependentModelWeeklyLimit(t *testing.T) {
	truncate(t)

	info := &relaycommon.RelayInfo{
		UserId:                310,
		OriginModelName:       "gpt-special",
		DailyTokenLimit:       20_000,
		WeeklyTokenLimit:      1,
		ModelWeeklyTokenLimit: 15_000,
		TokenMultiplier:       2,
		StartTime:             time.Date(2026, time.August, 7, 12, 0, 0, 0, time.Local),
	}
	require.Nil(t, PreConsumeDailyTokens(info, 2_500, 2_500))
	assert.EqualValues(t, 10_000, dailyTokenUsageForServiceTest(t, 310, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 10_000, modelWeeklyTokenUsageForServiceTest(t, 310, "gpt-special", "2026-08-03").UsedTokens)
	require.Nil(t, info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"fast"}`)))
	assert.EqualValues(t, 15_000, dailyTokenUsageForServiceTest(t, 310, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 15_000, modelWeeklyTokenUsageForServiceTest(t, 310, "gpt-special", "2026-08-03").UsedTokens)

	var generalWeeklyRows int64
	require.NoError(t, model.DB.Model(&model.UserWeeklyTokenUsage{}).Where("user_id = ?", 310).Count(&generalWeeklyRows).Error)
	assert.Zero(t, generalWeeklyRows)
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

func TestEstimateRequestTokenStillCountsForWeeklyLimitWhenGlobalCountingIsDisabled(t *testing.T) {
	oldCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() {
		constant.CountToken = oldCountToken
	})

	tokens, err := EstimateRequestToken(
		dailyTokenTestContext(),
		&types.TokenCountMeta{TokenType: types.TokenTypeTextNumber, CombineText: "每周限额"},
		&relaycommon.RelayInfo{WeeklyTokenLimit: 1_000},
	)
	require.NoError(t, err)
	assert.Equal(t, 4, tokens)
}

func TestEstimateRequestTokenStillCountsForModelWeeklyLimitWhenGlobalCountingIsDisabled(t *testing.T) {
	oldCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() {
		constant.CountToken = oldCountToken
	})

	tokens, err := EstimateRequestToken(
		dailyTokenTestContext(),
		&types.TokenCountMeta{TokenType: types.TokenTypeTextNumber, CombineText: "模型周限额"},
		&relaycommon.RelayInfo{ModelWeeklyTokenLimit: 1_000},
	)
	require.NoError(t, err)
	assert.Equal(t, 5, tokens)
}

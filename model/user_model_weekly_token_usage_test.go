package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func modelWeeklyTokenUsageForTest(t *testing.T, userId int, modelName string, weekStart string) UserModelWeeklyTokenUsage {
	t.Helper()
	var usage UserModelWeeklyTokenUsage
	require.NoError(t, DB.Where("user_id = ? AND model_name = ? AND week_start = ?", userId, modelName, weekStart).First(&usage).Error)
	return usage
}

func TestReserveUserModelWeeklyTokensStartsFreshOnNextWeek(t *testing.T) {
	truncateTables(t)

	require.NoError(t, ReserveUserModelWeeklyTokens(221, "gpt-special", "2026-08-03", 1_000, 1_000))
	require.ErrorIs(t, ReserveUserModelWeeklyTokens(221, "gpt-special", "2026-08-03", 1_000, 1), ErrModelWeeklyTokenLimitExceeded)
	require.NoError(t, ReserveUserModelWeeklyTokens(221, "gpt-special", "2026-08-10", 1_000, 1_000))

	assert.EqualValues(t, 1_000, modelWeeklyTokenUsageForTest(t, 221, "gpt-special", "2026-08-03").UsedTokens)
	assert.EqualValues(t, 1_000, modelWeeklyTokenUsageForTest(t, 221, "gpt-special", "2026-08-10").UsedTokens)
}

func TestReserveUserDailyAndModelWeeklyTokensIsAtomic(t *testing.T) {
	truncateTables(t)

	require.ErrorIs(
		t,
		ReserveUserDailyAndModelWeeklyTokens(222, "gpt-special", "2026-08-07", "2026-08-03", 1_000, 500, 600),
		ErrModelWeeklyTokenLimitExceeded,
	)

	var dailyRows int64
	require.NoError(t, DB.Model(&UserDailyTokenUsage{}).Where("user_id = ?", 222).Count(&dailyRows).Error)
	assert.Zero(t, dailyRows)
}

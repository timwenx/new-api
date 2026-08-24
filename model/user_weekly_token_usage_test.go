package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func weeklyTokenUsageForTest(t *testing.T, userId int, weekStart string) UserWeeklyTokenUsage {
	t.Helper()
	var usage UserWeeklyTokenUsage
	require.NoError(t, DB.Where("user_id = ? AND week_start = ?", userId, weekStart).First(&usage).Error)
	return usage
}

func TestWeeklyTokenUsageStartUsesMonday(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "monday", at: time.Date(2026, time.August, 3, 12, 0, 0, 0, time.Local), want: "2026-08-03"},
		{name: "sunday", at: time.Date(2026, time.August, 9, 23, 59, 0, 0, time.Local), want: "2026-08-03"},
		{name: "next monday", at: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.Local), want: "2026-08-10"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, WeeklyTokenUsageStart(test.at))
		})
	}
}

func TestPopulateUsersWeeklyTokenRemaining(t *testing.T) {
	truncateTables(t)

	const weekStart = "2026-08-03"
	users := []*User{
		{Id: 211, WeeklyTokenLimit: 1_000},
		{Id: 212, WeeklyTokenLimit: 0},
		{Id: 213, WeeklyTokenLimit: 100},
		{Id: 214, WeeklyTokenLimit: 200},
	}
	require.NoError(t, DB.Create(&[]UserWeeklyTokenUsage{
		{UserId: 211, WeekStart: weekStart, UsedTokens: 250},
		{UserId: 212, WeekStart: weekStart, UsedTokens: 50},
		{UserId: 213, WeekStart: weekStart, UsedTokens: 150},
	}).Error)

	require.NoError(t, PopulateUsersWeeklyTokenRemaining(users, weekStart))
	assert.EqualValues(t, 750, users[0].WeeklyTokenRemaining)
	assert.Zero(t, users[1].WeeklyTokenRemaining)
	assert.Zero(t, users[2].WeeklyTokenRemaining)
	assert.EqualValues(t, 200, users[3].WeeklyTokenRemaining)
}

func TestReserveUserWeeklyTokensStartsFreshOnNextWeek(t *testing.T) {
	truncateTables(t)

	require.NoError(t, ReserveUserWeeklyTokens(215, "2026-08-03", 1_000, 1_000))
	require.ErrorIs(t, ReserveUserWeeklyTokens(215, "2026-08-03", 1_000, 1), ErrWeeklyTokenLimitExceeded)
	require.NoError(t, ReserveUserWeeklyTokens(215, "2026-08-10", 1_000, 1_000))

	assert.EqualValues(t, 1_000, weeklyTokenUsageForTest(t, 215, "2026-08-03").UsedTokens)
	assert.EqualValues(t, 1_000, weeklyTokenUsageForTest(t, 215, "2026-08-10").UsedTokens)
}

func TestReserveUserTokenLimitsIsAtomicAndSettlesBothCounters(t *testing.T) {
	truncateTables(t)

	const (
		userId    = 216
		usageDate = "2026-08-07"
		weekStart = "2026-08-03"
	)
	require.ErrorIs(
		t,
		ReserveUserTokenLimits(userId, usageDate, weekStart, 1_000, 500, 600),
		ErrWeeklyTokenLimitExceeded,
	)

	var dailyRows int64
	require.NoError(t, DB.Model(&UserDailyTokenUsage{}).Where("user_id = ?", userId).Count(&dailyRows).Error)
	assert.Zero(t, dailyRows)

	require.NoError(t, ReserveUserTokenLimits(userId, usageDate, weekStart, 1_000, 500, 400))
	require.NoError(t, AdjustUserTokenLimits(userId, usageDate, weekStart, true, true, -100))
	assert.EqualValues(t, 300, dailyTokenUsageForTest(t, userId, usageDate).UsedTokens)
	assert.EqualValues(t, 300, weeklyTokenUsageForTest(t, userId, weekStart).UsedTokens)
}

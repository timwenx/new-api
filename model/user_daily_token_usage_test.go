package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dailyTokenUsageForTest(t *testing.T, userId int, usageDate string) UserDailyTokenUsage {
	t.Helper()
	var usage UserDailyTokenUsage
	require.NoError(t, DB.Where("user_id = ? AND usage_date = ?", userId, usageDate).First(&usage).Error)
	return usage
}

func TestReserveUserDailyTokensTreatsZeroLimitAsUnlimited(t *testing.T) {
	truncateTables(t)

	require.NoError(t, ReserveUserDailyTokens(101, "2026-08-07", 0, 10_000))

	var count int64
	require.NoError(t, DB.Model(&UserDailyTokenUsage{}).Where("user_id = ?", 101).Count(&count).Error)
	assert.Zero(t, count)
}

func TestPopulateUsersDailyTokenRemaining(t *testing.T) {
	truncateTables(t)

	const usageDate = "2026-08-07"
	users := []*User{
		{Id: 201, DailyTokenLimit: 1_000},
		{Id: 202, DailyTokenLimit: 0},
		{Id: 203, DailyTokenLimit: 100},
		{Id: 204, DailyTokenLimit: 200},
	}
	require.NoError(t, DB.Create(&[]UserDailyTokenUsage{
		{UserId: 201, UsageDate: usageDate, UsedTokens: 250},
		{UserId: 202, UsageDate: usageDate, UsedTokens: 50},
		{UserId: 203, UsageDate: usageDate, UsedTokens: 150},
	}).Error)

	require.NoError(t, PopulateUsersDailyTokenRemaining(users, usageDate))
	assert.EqualValues(t, 750, users[0].DailyTokenRemaining)
	assert.Zero(t, users[1].DailyTokenRemaining)
	assert.Zero(t, users[2].DailyTokenRemaining)
	assert.EqualValues(t, 200, users[3].DailyTokenRemaining)
}

func TestReserveUserDailyTokensEnforcesLimitAndSupportsSettlement(t *testing.T) {
	truncateTables(t)

	const usageDate = "2026-08-07"
	require.NoError(t, ReserveUserDailyTokens(102, usageDate, 1_000, 600))
	require.NoError(t, ReserveUserDailyTokens(102, usageDate, 1_000, 400))
	require.ErrorIs(t, ReserveUserDailyTokens(102, usageDate, 1_000, 1), ErrDailyTokenLimitExceeded)

	require.NoError(t, AdjustUserDailyTokens(102, usageDate, -100))
	require.NoError(t, ReserveUserDailyTokens(102, usageDate, 1_000, 100))
	assert.EqualValues(t, 1_000, dailyTokenUsageForTest(t, 102, usageDate).UsedTokens)
}

func TestAdjustUserDailyTokensReleasesReservationWithoutUnderflow(t *testing.T) {
	truncateTables(t)

	const usageDate = "2026-08-07"
	require.NoError(t, ReserveUserDailyTokens(103, usageDate, 1_000, 500))
	require.NoError(t, AdjustUserDailyTokens(103, usageDate, -500))
	assert.Zero(t, dailyTokenUsageForTest(t, 103, usageDate).UsedTokens)
	require.Error(t, AdjustUserDailyTokens(103, usageDate, -1))
}

func TestReserveUserDailyTokensStartsFreshOnNextDate(t *testing.T) {
	truncateTables(t)

	require.NoError(t, ReserveUserDailyTokens(104, "2026-08-07", 1_000, 1_000))
	require.ErrorIs(t, ReserveUserDailyTokens(104, "2026-08-07", 1_000, 1), ErrDailyTokenLimitExceeded)
	require.NoError(t, ReserveUserDailyTokens(104, "2026-08-08", 1_000, 1_000))

	assert.EqualValues(t, 1_000, dailyTokenUsageForTest(t, 104, "2026-08-07").UsedTokens)
	assert.EqualValues(t, 1_000, dailyTokenUsageForTest(t, 104, "2026-08-08").UsedTokens)
}

func TestReserveUserDailyTokensIsAtomicAcrossConcurrentRequests(t *testing.T) {
	truncateTables(t)

	const (
		userId          = 105
		usageDate       = "2026-08-07"
		limit           = 1_000
		reservation     = 100
		requestAttempts = 20
	)

	results := make(chan error, requestAttempts)
	var wg sync.WaitGroup
	for i := 0; i < requestAttempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- ReserveUserDailyTokens(userId, usageDate, limit, reservation)
		}()
	}
	wg.Wait()
	close(results)

	var accepted int
	var rejected int
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrDailyTokenLimitExceeded):
			rejected++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 10, accepted)
	assert.Equal(t, 10, rejected)
	assert.EqualValues(t, limit, dailyTokenUsageForTest(t, userId, usageDate).UsedTokens)
}

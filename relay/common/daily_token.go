package common

import "github.com/QuantumNous/new-api/types"

// FastTokenMultiplier is applied after the configured model token multiplier
// when the final upstream request uses the fast service tier.
const FastTokenMultiplier = 1.5

// DailyTokenSettler tracks the lifecycle of one request's account-level token
// limit reservations. It is implemented by service.DailyTokenSession.
type DailyTokenSettler interface {
	SetUsageMultiplier(multiplier float64) *types.NewAPIError
	Settle(actualTokens int) error
	Refund() error
}

package common

// DailyTokenSettler tracks the lifecycle of one request's account-level token
// limit reservations. It is implemented by service.DailyTokenSession.
type DailyTokenSettler interface {
	Settle(actualTokens int) error
	Refund() error
}

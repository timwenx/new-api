package common

// DailyTokenSettler tracks the lifecycle of one request's daily token
// reservation. It is implemented by service.DailyTokenSession.
type DailyTokenSettler interface {
	Settle(actualTokens int) error
	Refund() error
}

package service

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// DailyTokenSession owns one request's reservation on a site-local date.
type DailyTokenSession struct {
	userId         int
	usageDate      string
	reservedTokens int64
	settled        bool
	refunded       bool
	mu             sync.Mutex
}

func (s *DailyTokenSession) Settle(actualTokens int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded {
		return nil
	}
	if actualTokens < 0 {
		return errors.New("actual daily token usage cannot be negative")
	}
	delta := int64(actualTokens) - s.reservedTokens
	if err := model.AdjustUserDailyTokens(s.userId, s.usageDate, delta); err != nil {
		return err
	}
	s.settled = true
	return nil
}

func (s *DailyTokenSession) Refund() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded {
		return nil
	}
	if err := model.AdjustUserDailyTokens(s.userId, s.usageDate, -s.reservedTokens); err != nil {
		return err
	}
	s.refunded = true
	return nil
}

func dailyTokenReservationTokens(promptTokens int, maxOutputTokens int) int64 {
	prompt := int64(promptTokens)
	if prompt < 0 {
		prompt = 0
	}
	if maxOutputTokens > 0 {
		return prompt + int64(maxOutputTokens)
	}
	fallback := int64(common.PreConsumedQuota)
	if fallback < 1 {
		fallback = 1
	}
	if prompt > fallback {
		return prompt
	}
	return fallback
}

// PreConsumeDailyTokens reserves the request's estimated maximum usage before
// it reaches an upstream channel. The settled count is corrected to actual
// input + output tokens after a successful response.
func PreConsumeDailyTokens(relayInfo *relaycommon.RelayInfo, promptTokens int, maxOutputTokens int) *types.NewAPIError {
	if relayInfo == nil || relayInfo.DailyTokenLimit == 0 {
		return nil
	}
	if relayInfo.DailyTokenLimit < 0 || relayInfo.DailyTokenLimit > model.MaxDailyTokenLimit {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid daily token limit: %d", relayInfo.DailyTokenLimit),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	reservedTokens := dailyTokenReservationTokens(promptTokens, maxOutputTokens)
	requestTime := relayInfo.StartTime
	if requestTime.IsZero() {
		requestTime = time.Now()
	}
	usageDate := requestTime.In(time.Local).Format(time.DateOnly)
	err := model.ReserveUserDailyTokens(relayInfo.UserId, usageDate, relayInfo.DailyTokenLimit, reservedTokens)
	if errors.Is(err, model.ErrDailyTokenLimitExceeded) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("每日 Token 使用量已达到限额（%d），将在站点时区 00:00 重置", relayInfo.DailyTokenLimit),
			types.ErrorCodeDailyTokenLimitExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	if err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}

	relayInfo.DailyTokens = &DailyTokenSession{
		userId:         relayInfo.UserId,
		usageDate:      usageDate,
		reservedTokens: reservedTokens,
	}
	return nil
}

// SettleDailyTokens replaces the reservation with the response's actual usage.
func SettleDailyTokens(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualTokens int) {
	if relayInfo == nil || relayInfo.DailyTokens == nil {
		return
	}
	if err := relayInfo.DailyTokens.Settle(actualTokens); err != nil {
		logger.LogError(ctx, fmt.Sprintf("error settling daily token usage: %s", err.Error()))
	}
}

// RefundDailyTokens releases a reservation after a failed request.
func RefundDailyTokens(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil || relayInfo.DailyTokens == nil {
		return
	}
	if err := relayInfo.DailyTokens.Refund(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("error refunding daily token reservation: %s", err.Error()))
	}
}

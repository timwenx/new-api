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

// DailyTokenSession owns one request's daily and weekly token reservations.
type DailyTokenSession struct {
	userId             int
	modelName          string
	usageDate          string
	weekStart          string
	dailyEnabled       bool
	weeklyEnabled      bool
	modelWeeklyEnabled bool
	reservedTokens     int64
	settled            bool
	refunded           bool
	mu                 sync.Mutex
}

func (s *DailyTokenSession) Settle(actualTokens int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded {
		return nil
	}
	if actualTokens < 0 {
		return errors.New("actual token usage cannot be negative")
	}
	delta := int64(actualTokens) - s.reservedTokens
	if err := s.adjust(delta); err != nil {
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
	if err := s.adjust(-s.reservedTokens); err != nil {
		return err
	}
	s.refunded = true
	return nil
}

func (s *DailyTokenSession) adjust(delta int64) error {
	if s.modelWeeklyEnabled {
		return model.AdjustUserDailyAndModelWeeklyTokens(
			s.userId,
			s.modelName,
			s.usageDate,
			s.weekStart,
			s.dailyEnabled,
			true,
			delta,
		)
	}
	return model.AdjustUserTokenLimits(s.userId, s.usageDate, s.weekStart, s.dailyEnabled, s.weeklyEnabled, delta)
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

// PreConsumeDailyTokens reserves the request's estimated maximum usage against
// enabled daily and weekly limits before it reaches an upstream channel. The
// settled count is corrected to actual input + output tokens after success.
func PreConsumeDailyTokens(relayInfo *relaycommon.RelayInfo, promptTokens int, maxOutputTokens int) *types.NewAPIError {
	if relayInfo == nil || (relayInfo.DailyTokenLimit == 0 && relayInfo.WeeklyTokenLimit == 0 && relayInfo.ModelWeeklyTokenLimit == 0) {
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
	if relayInfo.WeeklyTokenLimit < 0 || relayInfo.WeeklyTokenLimit > model.MaxWeeklyTokenLimit {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid weekly token limit: %d", relayInfo.WeeklyTokenLimit),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if relayInfo.ModelWeeklyTokenLimit < 0 || relayInfo.ModelWeeklyTokenLimit > model.MaxWeeklyTokenLimit {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid model weekly token limit: %d", relayInfo.ModelWeeklyTokenLimit),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if relayInfo.ModelWeeklyTokenLimit > 0 && relayInfo.OriginModelName == "" {
		return types.NewErrorWithStatusCode(
			errors.New("model weekly token limit requires an original model name"),
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
	weekStart := model.WeeklyTokenUsageStart(requestTime)
	var err error
	if relayInfo.ModelWeeklyTokenLimit > 0 {
		err = model.ReserveUserDailyAndModelWeeklyTokens(
			relayInfo.UserId,
			relayInfo.OriginModelName,
			usageDate,
			weekStart,
			relayInfo.DailyTokenLimit,
			relayInfo.ModelWeeklyTokenLimit,
			reservedTokens,
		)
	} else {
		err = model.ReserveUserTokenLimits(
			relayInfo.UserId,
			usageDate,
			weekStart,
			relayInfo.DailyTokenLimit,
			relayInfo.WeeklyTokenLimit,
			reservedTokens,
		)
	}
	if errors.Is(err, model.ErrDailyTokenLimitExceeded) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("每日 Token 使用量已达到限额（%d），将在站点时区 00:00 重置", relayInfo.DailyTokenLimit),
			types.ErrorCodeDailyTokenLimitExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	if errors.Is(err, model.ErrWeeklyTokenLimitExceeded) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("每周 Token 使用量已达到限额（%d），将在站点时区下周一 00:00 重置", relayInfo.WeeklyTokenLimit),
			types.ErrorCodeWeeklyTokenLimitExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	if errors.Is(err, model.ErrModelWeeklyTokenLimitExceeded) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("模型 %s 每周 Token 使用量已达到独立限额（%d），将在站点时区下周一 00:00 重置", relayInfo.OriginModelName, relayInfo.ModelWeeklyTokenLimit),
			types.ErrorCodeModelWeeklyTokenLimitExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	if err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}

	relayInfo.DailyTokens = &DailyTokenSession{
		userId:             relayInfo.UserId,
		modelName:          relayInfo.OriginModelName,
		usageDate:          usageDate,
		weekStart:          weekStart,
		dailyEnabled:       relayInfo.DailyTokenLimit > 0,
		weeklyEnabled:      relayInfo.WeeklyTokenLimit > 0 && relayInfo.ModelWeeklyTokenLimit == 0,
		modelWeeklyEnabled: relayInfo.ModelWeeklyTokenLimit > 0,
		reservedTokens:     reservedTokens,
	}
	return nil
}

// SettleDailyTokens replaces enabled limit reservations with the response's actual usage.
func SettleDailyTokens(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualTokens int) {
	if relayInfo == nil || relayInfo.DailyTokens == nil {
		return
	}
	if err := relayInfo.DailyTokens.Settle(actualTokens); err != nil {
		logger.LogError(ctx, fmt.Sprintf("error settling token limit usage: %s", err.Error()))
	}
}

// RefundDailyTokens releases enabled limit reservations after a failed request.
func RefundDailyTokens(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil || relayInfo.DailyTokens == nil {
		return
	}
	if err := relayInfo.DailyTokens.Refund(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("error refunding token limit reservation: %s", err.Error()))
	}
}

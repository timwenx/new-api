package service

import (
	"errors"
	"fmt"
	"math"
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
	dailyLimit         int64
	weeklyLimit        int64
	modelWeeklyLimit   int64
	modelMultiplier    float64
	usageMultiplier    float64
	rawReservedTokens  int64
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
	countedTokens, err := common.ScaleTokenCount(int64(actualTokens), s.modelMultiplier*s.usageMultiplier, model.MaxWeeklyTokenLimit)
	if err != nil {
		return fmt.Errorf("error applying token multipliers: %w", err)
	}
	delta := countedTokens - s.reservedTokens
	if err := s.adjust(delta); err != nil {
		return err
	}
	s.settled = true
	return nil
}

// SetUsageMultiplier updates the reservation after the final outbound request
// determines whether fast mode is actually enabled.
func (s *DailyTokenSession) SetUsageMultiplier(multiplier float64) *types.NewAPIError {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded {
		return nil
	}

	targetTokens, err := common.ScaleTokenCount(s.rawReservedTokens, s.modelMultiplier*multiplier, model.MaxWeeklyTokenLimit)
	if err != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid usage token multiplier: %w", err),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	delta := targetTokens - s.reservedTokens
	if delta > 0 {
		err = s.reserve(delta)
	} else {
		err = s.adjust(delta)
	}
	if err != nil {
		return tokenLimitAPIError(err, s.dailyLimit, s.weeklyLimit, s.modelName, s.modelWeeklyLimit)
	}

	s.usageMultiplier = multiplier
	s.reservedTokens = targetTokens
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

func (s *DailyTokenSession) reserve(tokens int64) error {
	if s.modelWeeklyEnabled {
		return model.ReserveUserDailyAndModelWeeklyTokens(
			s.userId,
			s.modelName,
			s.usageDate,
			s.weekStart,
			s.dailyLimit,
			s.modelWeeklyLimit,
			tokens,
		)
	}
	return model.ReserveUserTokenLimits(
		s.userId,
		s.usageDate,
		s.weekStart,
		s.dailyLimit,
		s.weeklyLimit,
		tokens,
	)
}

func tokenLimitAPIError(err error, dailyLimit int64, weeklyLimit int64, modelName string, modelWeeklyLimit int64) *types.NewAPIError {
	if errors.Is(err, model.ErrDailyTokenLimitExceeded) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("每日 Token 使用量已达到限额（%d），将在站点时区 00:00 重置", dailyLimit),
			types.ErrorCodeDailyTokenLimitExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	if errors.Is(err, model.ErrWeeklyTokenLimitExceeded) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("每周 Token 使用量已达到限额（%d），将在站点时区下周一 00:00 重置", weeklyLimit),
			types.ErrorCodeWeeklyTokenLimitExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	if errors.Is(err, model.ErrModelWeeklyTokenLimitExceeded) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("模型 %s 每周 Token 使用量已达到独立限额（%d），将在站点时区下周一 00:00 重置", modelName, modelWeeklyLimit),
			types.ErrorCodeModelWeeklyTokenLimitExceeded,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	if err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
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

func relayTokenMultiplier(relayInfo *relaycommon.RelayInfo) float64 {
	if relayInfo == nil || relayInfo.TokenMultiplier <= 0 || math.IsNaN(relayInfo.TokenMultiplier) || math.IsInf(relayInfo.TokenMultiplier, 0) {
		return 1
	}
	return relayInfo.TokenMultiplier
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

	tokenMultiplier := relayTokenMultiplier(relayInfo)
	rawReservedTokens := dailyTokenReservationTokens(promptTokens, maxOutputTokens)
	reservedTokens, scaleErr := common.ScaleTokenCount(
		rawReservedTokens,
		tokenMultiplier,
		model.MaxWeeklyTokenLimit,
	)
	if scaleErr != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid model token multiplier: %w", scaleErr),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	requestTime := relayInfo.StartTime
	if requestTime.IsZero() {
		requestTime = time.Now()
	}
	usageDate := requestTime.In(time.Local).Format(time.DateOnly)
	weekStart := model.WeeklyTokenUsageStart(requestTime)
	session := &DailyTokenSession{
		userId:             relayInfo.UserId,
		modelName:          relayInfo.OriginModelName,
		usageDate:          usageDate,
		weekStart:          weekStart,
		dailyEnabled:       relayInfo.DailyTokenLimit > 0,
		weeklyEnabled:      relayInfo.WeeklyTokenLimit > 0 && relayInfo.ModelWeeklyTokenLimit == 0,
		modelWeeklyEnabled: relayInfo.ModelWeeklyTokenLimit > 0,
		dailyLimit:         relayInfo.DailyTokenLimit,
		weeklyLimit:        relayInfo.WeeklyTokenLimit,
		modelWeeklyLimit:   relayInfo.ModelWeeklyTokenLimit,
		modelMultiplier:    tokenMultiplier,
		usageMultiplier:    1,
		rawReservedTokens:  rawReservedTokens,
		reservedTokens:     reservedTokens,
	}
	if err := session.reserve(reservedTokens); err != nil {
		return tokenLimitAPIError(err, relayInfo.DailyTokenLimit, relayInfo.WeeklyTokenLimit, relayInfo.OriginModelName, relayInfo.ModelWeeklyTokenLimit)
	}
	relayInfo.DailyTokens = session
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

package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrModelWeeklyTokenLimitExceeded indicates that a reservation would exceed
// the configured model's independent weekly limit.
var ErrModelWeeklyTokenLimitExceeded = errors.New("model weekly token limit exceeded")

// UserModelWeeklyTokenUsage stores the independent weekly usage bucket for the
// single model configured in system settings.
type UserModelWeeklyTokenUsage struct {
	Id         int64  `json:"id"`
	UserId     int    `json:"user_id" gorm:"not null;uniqueIndex:idx_user_model_weekly_token_usage"`
	ModelName  string `json:"model_name" gorm:"type:varchar(191);not null;uniqueIndex:idx_user_model_weekly_token_usage"`
	WeekStart  string `json:"week_start" gorm:"type:varchar(10);not null;uniqueIndex:idx_user_model_weekly_token_usage"`
	UsedTokens int64  `json:"used_tokens" gorm:"not null"`
}

func ReserveUserModelWeeklyTokens(userId int, modelName string, weekStart string, limit int64, tokens int64) error {
	return reserveUserModelWeeklyTokens(DB, userId, modelName, weekStart, limit, tokens)
}

func reserveUserModelWeeklyTokens(tx *gorm.DB, userId int, modelName string, weekStart string, limit int64, tokens int64) error {
	if userId <= 0 || modelName == "" || weekStart == "" {
		return errors.New("invalid model weekly token usage identity")
	}
	if limit < 0 || tokens <= 0 {
		return errors.New("invalid model weekly token reservation")
	}
	if limit == 0 {
		return nil
	}
	if tokens > limit {
		return ErrModelWeeklyTokenLimitExceeded
	}

	usage := UserModelWeeklyTokenUsage{
		UserId:    userId,
		ModelName: modelName,
		WeekStart: weekStart,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage).Error; err != nil {
		return err
	}

	result := tx.Model(&UserModelWeeklyTokenUsage{}).
		Where("user_id = ? AND model_name = ? AND week_start = ?", userId, modelName, weekStart).
		Where("used_tokens <= ?", limit-tokens).
		UpdateColumn("used_tokens", gorm.Expr("used_tokens + ?", tokens))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrModelWeeklyTokenLimitExceeded
	}
	return nil
}

func AdjustUserModelWeeklyTokens(userId int, modelName string, weekStart string, delta int64) error {
	return adjustUserModelWeeklyTokens(DB, userId, modelName, weekStart, delta)
}

func adjustUserModelWeeklyTokens(tx *gorm.DB, userId int, modelName string, weekStart string, delta int64) error {
	if userId <= 0 || modelName == "" || weekStart == "" {
		return errors.New("invalid model weekly token usage identity")
	}
	if delta == 0 {
		return nil
	}

	query := tx.Model(&UserModelWeeklyTokenUsage{}).
		Where("user_id = ? AND model_name = ? AND week_start = ?", userId, modelName, weekStart)
	if delta < 0 {
		query = query.Where("used_tokens >= ?", -delta)
	}
	result := query.UpdateColumn("used_tokens", gorm.Expr("used_tokens + ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("model weekly token usage row not found or adjustment would underflow")
	}
	return nil
}

// ReserveUserDailyAndModelWeeklyTokens atomically reserves the user's daily
// bucket and the configured model's independent weekly bucket.
func ReserveUserDailyAndModelWeeklyTokens(userId int, modelName string, usageDate string, weekStart string, dailyLimit int64, modelWeeklyLimit int64, tokens int64) error {
	switch {
	case dailyLimit == 0:
		return ReserveUserModelWeeklyTokens(userId, modelName, weekStart, modelWeeklyLimit, tokens)
	case modelWeeklyLimit == 0:
		return ReserveUserDailyTokens(userId, usageDate, dailyLimit, tokens)
	default:
		return DB.Transaction(func(tx *gorm.DB) error {
			if err := reserveUserDailyTokens(tx, userId, usageDate, dailyLimit, tokens); err != nil {
				return err
			}
			return reserveUserModelWeeklyTokens(tx, userId, modelName, weekStart, modelWeeklyLimit, tokens)
		})
	}
}

// AdjustUserDailyAndModelWeeklyTokens settles or refunds the two reservations
// as one transaction.
func AdjustUserDailyAndModelWeeklyTokens(userId int, modelName string, usageDate string, weekStart string, adjustDaily bool, adjustModelWeekly bool, delta int64) error {
	switch {
	case !adjustDaily && !adjustModelWeekly:
		return nil
	case !adjustDaily:
		return AdjustUserModelWeeklyTokens(userId, modelName, weekStart, delta)
	case !adjustModelWeekly:
		return AdjustUserDailyTokens(userId, usageDate, delta)
	default:
		return DB.Transaction(func(tx *gorm.DB) error {
			if err := adjustUserDailyTokens(tx, userId, usageDate, delta); err != nil {
				return err
			}
			return adjustUserModelWeeklyTokens(tx, userId, modelName, weekStart, delta)
		})
	}
}

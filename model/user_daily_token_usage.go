package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MaxDailyTokenLimit keeps administrator input within the application's 32-bit accounting boundary.
const MaxDailyTokenLimit int64 = 2_147_483_647

// ErrDailyTokenLimitExceeded indicates that a reservation would exceed today's limit.
var ErrDailyTokenLimitExceeded = errors.New("daily token limit exceeded")

// UserDailyTokenUsage stores a user's reserved and settled token usage for one
// site-local calendar day. A new date starts with an independent counter.
type UserDailyTokenUsage struct {
	Id         int64  `json:"id"`
	UserId     int    `json:"user_id" gorm:"not null;uniqueIndex:idx_user_daily_token_usage"`
	UsageDate  string `json:"usage_date" gorm:"type:varchar(10);not null;uniqueIndex:idx_user_daily_token_usage"`
	UsedTokens int64  `json:"used_tokens" gorm:"not null"`
}

// PopulateUsersDailyTokenRemaining fills the remaining allowance for a site-local date.
// Users without a daily limit have a remaining value of zero.
func PopulateUsersDailyTokenRemaining(users []*User, usageDate string) error {
	if usageDate == "" {
		return errors.New("invalid daily token usage date")
	}

	limitedUserIds := make([]int, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		user.DailyTokenRemaining = 0
		if user.DailyTokenLimit > 0 {
			limitedUserIds = append(limitedUserIds, user.Id)
		}
	}
	if len(limitedUserIds) == 0 {
		return nil
	}

	var usages []UserDailyTokenUsage
	if err := DB.Select("user_id", "used_tokens").
		Where("usage_date = ? AND user_id IN ?", usageDate, limitedUserIds).
		Find(&usages).Error; err != nil {
		return err
	}

	usedTokensByUser := make(map[int]int64, len(usages))
	for _, usage := range usages {
		usedTokensByUser[usage.UserId] = usage.UsedTokens
	}
	for _, user := range users {
		if user == nil || user.DailyTokenLimit <= 0 {
			continue
		}
		remaining := user.DailyTokenLimit - usedTokensByUser[user.Id]
		if remaining > 0 {
			user.DailyTokenRemaining = remaining
		}
	}
	return nil
}

// UpdateUserDailyTokenLimitWithTx updates the administrator-controlled user limit.
func UpdateUserDailyTokenLimitWithTx(tx *gorm.DB, userId int, limit int64) error {
	if userId <= 0 || limit < 0 || limit > MaxDailyTokenLimit {
		return errors.New("invalid daily token limit")
	}
	return tx.Model(&User{}).Where("id = ?", userId).Update("daily_token_limit", limit).Error
}

// ReserveUserDailyTokens atomically reserves tokens without allowing the
// date's counter to exceed limit. A zero limit means unlimited usage.
func ReserveUserDailyTokens(userId int, usageDate string, limit int64, tokens int64) error {
	if userId <= 0 || usageDate == "" {
		return errors.New("invalid daily token usage identity")
	}
	if limit < 0 || tokens <= 0 {
		return errors.New("invalid daily token reservation")
	}
	if limit == 0 {
		return nil
	}
	if tokens > limit {
		return ErrDailyTokenLimitExceeded
	}

	usage := UserDailyTokenUsage{
		UserId:    userId,
		UsageDate: usageDate,
	}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage).Error; err != nil {
		return err
	}

	result := DB.Model(&UserDailyTokenUsage{}).
		Where("user_id = ? AND usage_date = ?", userId, usageDate).
		Where("used_tokens <= ?", limit-tokens).
		UpdateColumn("used_tokens", gorm.Expr("used_tokens + ?", tokens))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDailyTokenLimitExceeded
	}
	return nil
}

// AdjustUserDailyTokens settles a reservation to the actual token count or
// releases it after a failed request. Negative adjustments cannot underflow.
func AdjustUserDailyTokens(userId int, usageDate string, delta int64) error {
	if userId <= 0 || usageDate == "" {
		return errors.New("invalid daily token usage identity")
	}
	if delta == 0 {
		return nil
	}

	query := DB.Model(&UserDailyTokenUsage{}).
		Where("user_id = ? AND usage_date = ?", userId, usageDate)
	if delta < 0 {
		query = query.Where("used_tokens >= ?", -delta)
	}
	result := query.UpdateColumn("used_tokens", gorm.Expr("used_tokens + ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("daily token usage row not found or adjustment would underflow")
	}
	return nil
}

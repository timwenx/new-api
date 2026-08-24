package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MaxWeeklyTokenLimit keeps administrator input within the application's 32-bit accounting boundary.
const MaxWeeklyTokenLimit int64 = MaxDailyTokenLimit

// ErrWeeklyTokenLimitExceeded indicates that a reservation would exceed the current week's limit.
var ErrWeeklyTokenLimitExceeded = errors.New("weekly token limit exceeded")

// UserWeeklyTokenUsage stores a user's reserved and settled token usage for one
// site-local calendar week beginning on Monday.
type UserWeeklyTokenUsage struct {
	Id         int64  `json:"id"`
	UserId     int    `json:"user_id" gorm:"not null;uniqueIndex:idx_user_weekly_token_usage"`
	WeekStart  string `json:"week_start" gorm:"type:varchar(10);not null;uniqueIndex:idx_user_weekly_token_usage"`
	UsedTokens int64  `json:"used_tokens" gorm:"not null"`
}

// WeeklyTokenUsageStart returns the Monday that starts the site-local week.
func WeeklyTokenUsageStart(at time.Time) string {
	localTime := at.In(time.Local)
	daysSinceMonday := (int(localTime.Weekday()) + 6) % 7
	return localTime.AddDate(0, 0, -daysSinceMonday).Format(time.DateOnly)
}

// PopulateUsersWeeklyTokenRemaining fills the remaining allowance for a site-local week.
// Users without a weekly limit have a remaining value of zero.
func PopulateUsersWeeklyTokenRemaining(users []*User, weekStart string) error {
	if weekStart == "" {
		return errors.New("invalid weekly token usage start")
	}

	limitedUserIds := make([]int, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		user.WeeklyTokenRemaining = 0
		if user.WeeklyTokenLimit > 0 {
			limitedUserIds = append(limitedUserIds, user.Id)
		}
	}
	if len(limitedUserIds) == 0 {
		return nil
	}

	var usages []UserWeeklyTokenUsage
	if err := DB.Select("user_id", "used_tokens").
		Where("week_start = ? AND user_id IN ?", weekStart, limitedUserIds).
		Find(&usages).Error; err != nil {
		return err
	}

	usedTokensByUser := make(map[int]int64, len(usages))
	for _, usage := range usages {
		usedTokensByUser[usage.UserId] = usage.UsedTokens
	}
	for _, user := range users {
		if user == nil || user.WeeklyTokenLimit <= 0 {
			continue
		}
		remaining := user.WeeklyTokenLimit - usedTokensByUser[user.Id]
		if remaining > 0 {
			user.WeeklyTokenRemaining = remaining
		}
	}
	return nil
}

// UpdateUserWeeklyTokenLimitWithTx updates the administrator-controlled user limit.
func UpdateUserWeeklyTokenLimitWithTx(tx *gorm.DB, userId int, limit int64) error {
	if userId <= 0 || limit < 0 || limit > MaxWeeklyTokenLimit {
		return errors.New("invalid weekly token limit")
	}
	return tx.Model(&User{}).Where("id = ?", userId).Update("weekly_token_limit", limit).Error
}

// ReserveUserWeeklyTokens atomically reserves tokens without allowing the
// week's counter to exceed limit. A zero limit means unlimited usage.
func ReserveUserWeeklyTokens(userId int, weekStart string, limit int64, tokens int64) error {
	return reserveUserWeeklyTokens(DB, userId, weekStart, limit, tokens)
}

func reserveUserWeeklyTokens(tx *gorm.DB, userId int, weekStart string, limit int64, tokens int64) error {
	if userId <= 0 || weekStart == "" {
		return errors.New("invalid weekly token usage identity")
	}
	if limit < 0 || tokens <= 0 {
		return errors.New("invalid weekly token reservation")
	}
	if limit == 0 {
		return nil
	}
	if tokens > limit {
		return ErrWeeklyTokenLimitExceeded
	}

	usage := UserWeeklyTokenUsage{
		UserId:    userId,
		WeekStart: weekStart,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage).Error; err != nil {
		return err
	}

	result := tx.Model(&UserWeeklyTokenUsage{}).
		Where("user_id = ? AND week_start = ?", userId, weekStart).
		Where("used_tokens <= ?", limit-tokens).
		UpdateColumn("used_tokens", gorm.Expr("used_tokens + ?", tokens))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWeeklyTokenLimitExceeded
	}
	return nil
}

// AdjustUserWeeklyTokens settles a reservation to the actual token count or
// releases it after a failed request. Negative adjustments cannot underflow.
func AdjustUserWeeklyTokens(userId int, weekStart string, delta int64) error {
	return adjustUserWeeklyTokens(DB, userId, weekStart, delta)
}

func adjustUserWeeklyTokens(tx *gorm.DB, userId int, weekStart string, delta int64) error {
	if userId <= 0 || weekStart == "" {
		return errors.New("invalid weekly token usage identity")
	}
	if delta == 0 {
		return nil
	}

	query := tx.Model(&UserWeeklyTokenUsage{}).
		Where("user_id = ? AND week_start = ?", userId, weekStart)
	if delta < 0 {
		query = query.Where("used_tokens >= ?", -delta)
	}
	result := query.UpdateColumn("used_tokens", gorm.Expr("used_tokens + ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("weekly token usage row not found or adjustment would underflow")
	}
	return nil
}

// ReserveUserTokenLimits reserves enabled daily and weekly limits as one operation.
func ReserveUserTokenLimits(userId int, usageDate string, weekStart string, dailyLimit int64, weeklyLimit int64, tokens int64) error {
	switch {
	case dailyLimit == 0:
		return ReserveUserWeeklyTokens(userId, weekStart, weeklyLimit, tokens)
	case weeklyLimit == 0:
		return ReserveUserDailyTokens(userId, usageDate, dailyLimit, tokens)
	default:
		return DB.Transaction(func(tx *gorm.DB) error {
			if err := reserveUserDailyTokens(tx, userId, usageDate, dailyLimit, tokens); err != nil {
				return err
			}
			return reserveUserWeeklyTokens(tx, userId, weekStart, weeklyLimit, tokens)
		})
	}
}

// AdjustUserTokenLimits settles or refunds enabled daily and weekly reservations as one operation.
func AdjustUserTokenLimits(userId int, usageDate string, weekStart string, adjustDaily bool, adjustWeekly bool, delta int64) error {
	switch {
	case !adjustDaily && !adjustWeekly:
		return nil
	case !adjustDaily:
		return AdjustUserWeeklyTokens(userId, weekStart, delta)
	case !adjustWeekly:
		return AdjustUserDailyTokens(userId, usageDate, delta)
	default:
		return DB.Transaction(func(tx *gorm.DB) error {
			if err := adjustUserDailyTokens(tx, userId, usageDate, delta); err != nil {
				return err
			}
			return adjustUserWeeklyTokens(tx, userId, weekStart, delta)
		})
	}
}

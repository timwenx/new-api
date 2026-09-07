package operation_setting

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

// MaxModelTokenMultiplier bounds administrator-configured usage scaling.
const MaxModelTokenMultiplier = 1_000_000

// TokenSetting 令牌相关配置
type TokenSetting struct {
	MaxUserTokens         int                           `json:"max_user_tokens"`          // 每用户最大令牌数量
	ModelWeeklyLimitModel string                        `json:"model_weekly_limit_model"` // 独立周额度适用的原始模型名
	ModelWeeklyTokenLimit int64                         `json:"model_weekly_token_limit"` // 每个用户对该模型的独立周 Token 限额
	ModelTokenMultipliers *types.RWMap[string, float64] `json:"model_token_multipliers"`  // 按原始模型名统计 Token 的倍率
}

// 默认配置
var tokenSetting = TokenSetting{
	MaxUserTokens:         1000,          // 默认每用户最多 1000 个令牌
	ModelWeeklyTokenLimit: 1_000_000_000, // 模型配置为空时不启用
	ModelTokenMultipliers: types.NewRWMap[string, float64](),
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("token_setting", &tokenSetting)
}

// GetTokenSetting 获取令牌配置
func GetTokenSetting() *TokenSetting {
	return &tokenSetting
}

// GetMaxUserTokens 获取每用户最大令牌数量
func GetMaxUserTokens() int {
	return GetTokenSetting().MaxUserTokens
}

// GetModelWeeklyTokenLimit returns the configured per-user weekly limit when
// modelName exactly matches the original model requested by the client.
func GetModelWeeklyTokenLimit(modelName string) int64 {
	configuredModel := strings.TrimSpace(tokenSetting.ModelWeeklyLimitModel)
	if configuredModel == "" || modelName != configuredModel || tokenSetting.ModelWeeklyTokenLimit <= 0 {
		return 0
	}
	return tokenSetting.ModelWeeklyTokenLimit
}

// GetModelTokenMultiplier returns the usage multiplier configured for the
// exact original model name requested by the client.
func GetModelTokenMultiplier(modelName string) float64 {
	if tokenSetting.ModelTokenMultipliers == nil {
		return 1
	}
	multiplier, ok := tokenSetting.ModelTokenMultipliers.Get(strings.TrimSpace(modelName))
	if !ok || multiplier <= 0 || multiplier > MaxModelTokenMultiplier || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 1
	}
	return multiplier
}

// NormalizeModelTokenMultipliers validates and canonicalizes the JSON object
// stored by the system settings API.
func NormalizeModelTokenMultipliers(raw string) (string, error) {
	parsed := make(map[string]float64)
	if err := common.UnmarshalJsonStr(strings.TrimSpace(raw), &parsed); err != nil {
		return "", fmt.Errorf("模型 Token 统计倍率必须是 JSON 对象: %w", err)
	}
	if parsed == nil {
		return "", errors.New("模型 Token 统计倍率必须是 JSON 对象")
	}

	normalized := make(map[string]float64, len(parsed))
	for rawModel, multiplier := range parsed {
		modelName := strings.TrimSpace(rawModel)
		if modelName == "" {
			return "", errors.New("模型 ID 不能为空")
		}
		if utf8.RuneCountInString(modelName) > 191 {
			return "", fmt.Errorf("模型 ID 不能超过 191 个字符: %s", modelName)
		}
		if multiplier <= 0 || multiplier > MaxModelTokenMultiplier || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
			return "", fmt.Errorf("模型 %s 的 Token 统计倍率必须大于 0 且不超过 %d", modelName, MaxModelTokenMultiplier)
		}
		if _, exists := normalized[modelName]; exists {
			return "", fmt.Errorf("模型 ID 去除首尾空格后重复: %s", modelName)
		}
		normalized[modelName] = multiplier
	}

	encoded, err := common.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

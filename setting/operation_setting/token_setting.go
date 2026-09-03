package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// TokenSetting 令牌相关配置
type TokenSetting struct {
	MaxUserTokens         int    `json:"max_user_tokens"`          // 每用户最大令牌数量
	ModelWeeklyLimitModel string `json:"model_weekly_limit_model"` // 独立周额度适用的原始模型名
	ModelWeeklyTokenLimit int64  `json:"model_weekly_token_limit"` // 每个用户对该模型的独立周 Token 限额
}

// 默认配置
var tokenSetting = TokenSetting{
	MaxUserTokens:         1000,          // 默认每用户最多 1000 个令牌
	ModelWeeklyTokenLimit: 1_000_000_000, // 模型配置为空时不启用
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

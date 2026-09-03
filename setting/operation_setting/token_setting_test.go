package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetModelWeeklyTokenLimitRequiresExactConfiguredModel(t *testing.T) {
	previous := *GetTokenSetting()
	t.Cleanup(func() {
		*GetTokenSetting() = previous
	})

	settings := GetTokenSetting()
	settings.ModelWeeklyLimitModel = "gpt-special"
	settings.ModelWeeklyTokenLimit = 1_000_000_000

	assert.EqualValues(t, 1_000_000_000, GetModelWeeklyTokenLimit("gpt-special"))
	assert.Zero(t, GetModelWeeklyTokenLimit("GPT-SPECIAL"))
	assert.Zero(t, GetModelWeeklyTokenLimit("other-model"))

	settings.ModelWeeklyTokenLimit = 0
	assert.Zero(t, GetModelWeeklyTokenLimit("gpt-special"))
}

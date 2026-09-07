package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestModelTokenMultipliersNormalizeAndMatchExactModel(t *testing.T) {
	settings := GetTokenSetting()
	previous := settings.ModelTokenMultipliers.ReadAll()
	t.Cleanup(func() {
		settings.ModelTokenMultipliers.Clear()
		settings.ModelTokenMultipliers.AddAll(previous)
	})

	normalized, err := NormalizeModelTokenMultipliers(`{" gpt-special ":2,"gpt-small":0.5}`)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(settings, map[string]string{
		"model_token_multipliers": normalized,
	}))

	assert.Equal(t, 2.0, GetModelTokenMultiplier("gpt-special"))
	assert.Equal(t, 0.5, GetModelTokenMultiplier("gpt-small"))
	assert.Equal(t, 1.0, GetModelTokenMultiplier("GPT-SPECIAL"))
	assert.Equal(t, 1.0, GetModelTokenMultiplier("other-model"))

	exported, err := config.ConfigToMap(settings)
	require.NoError(t, err)
	assert.JSONEq(t, `{"gpt-special":2,"gpt-small":0.5}`, exported["model_token_multipliers"])
}

func TestNormalizeModelTokenMultipliersRejectsInvalidEntries(t *testing.T) {
	_, err := NormalizeModelTokenMultipliers(`null`)
	require.Error(t, err)

	_, err = NormalizeModelTokenMultipliers(`{"gpt-special":0}`)
	require.Error(t, err)

	_, err = NormalizeModelTokenMultipliers(`{"gpt-special":1000001}`)
	require.Error(t, err)

	_, err = NormalizeModelTokenMultipliers(`{"gpt-special":2," gpt-special ":3}`)
	require.Error(t, err)
}

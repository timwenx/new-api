package common

import (
	"net/http/httptest"
	"testing"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGenRelayInfoAppliesModelWeeklyLimitToExactOriginalModel(t *testing.T) {
	previous := *operation_setting.GetTokenSetting()
	t.Cleanup(func() {
		*operation_setting.GetTokenSetting() = previous
	})
	settings := operation_setting.GetTokenSetting()
	settings.ModelWeeklyLimitModel = "gpt-special"
	settings.ModelWeeklyTokenLimit = 1_000_000_000

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	appcommon.SetContextKey(ctx, constant.ContextKeyOriginalModel, "gpt-special")
	appcommon.SetContextKey(ctx, constant.ContextKeyUserWeeklyTokenLimit, int64(50_000))

	info := GenRelayInfoOpenAI(ctx, nil)
	assert.EqualValues(t, 1_000_000_000, info.ModelWeeklyTokenLimit)
	assert.EqualValues(t, 50_000, info.WeeklyTokenLimit)
}

func TestGenRelayInfoSnapshotsExactModelTokenMultiplier(t *testing.T) {
	settings := operation_setting.GetTokenSetting()
	previous := settings.ModelTokenMultipliers.ReadAll()
	t.Cleanup(func() {
		settings.ModelTokenMultipliers.Clear()
		settings.ModelTokenMultipliers.AddAll(previous)
	})
	settings.ModelTokenMultipliers.Clear()
	settings.ModelTokenMultipliers.Set("gpt-special", 2)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	appcommon.SetContextKey(ctx, constant.ContextKeyOriginalModel, "gpt-special")

	info := GenRelayInfoOpenAI(ctx, nil)
	assert.Equal(t, 2.0, info.TokenMultiplier)

	appcommon.SetContextKey(ctx, constant.ContextKeyOriginalModel, "GPT-SPECIAL")
	info = GenRelayInfoOpenAI(ctx, nil)
	assert.Equal(t, 1.0, info.TokenMultiplier)
}

package model

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLogsRecordIPByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)

	oldRedisEnabled := common.RedisEnabled
	oldLogConsumeEnabled := common.LogConsumeEnabled
	common.RedisEnabled = false
	common.LogConsumeEnabled = true
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.LogConsumeEnabled = oldLogConsumeEnabled
	})

	disabled := false
	tests := []struct {
		name    string
		setting dto.UserSetting
		wantIP  string
	}{
		{name: "setting omitted", wantIP: "203.0.113.10"},
		{
			name:    "setting explicitly disabled",
			setting: dto.UserSetting{RecordIpLog: &disabled},
			wantIP:  "",
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := User{
				Id:       index + 1,
				Username: fmt.Sprintf("ip-log-user-%d", index+1),
				Password: "password",
				Status:   common.UserStatusEnabled,
				AffCode:  fmt.Sprintf("ip-log-aff-%d", index+1),
			}
			user.SetSetting(tt.setting)
			require.NoError(t, DB.Create(&user).Error)

			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			ctx.Request.RemoteAddr = "203.0.113.10:12345"
			ctx.Set("username", user.Username)

			RecordConsumeLog(ctx, user.Id, RecordConsumeLogParams{ModelName: "gpt-test"})
			RecordErrorLog(ctx, user.Id, 0, "gpt-test", "", "request failed", 0, 0, false, "default", nil)

			var logs []Log
			require.NoError(t, DB.Where("user_id = ?", user.Id).Order("type").Find(&logs).Error)
			require.Len(t, logs, 2)
			for _, log := range logs {
				assert.Equal(t, tt.wantIP, log.Ip)
			}
		})
	}
}

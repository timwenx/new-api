package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserSettingShouldApplyIPLimit(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name    string
		setting UserSetting
		want    bool
	}{
		{name: "setting omitted", want: true},
		{name: "setting enabled", setting: UserSetting{IPLimitEnabled: &enabled}, want: true},
		{name: "setting disabled", setting: UserSetting{IPLimitEnabled: &disabled}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.setting.ShouldApplyIPLimit())
		})
	}
}

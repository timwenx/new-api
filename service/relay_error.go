package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

func ShouldRetryResponsesOnOriginalChannel(c *gin.Context) bool {
	return common.ResponsesSameChannelRetryEnabled &&
		c != nil && c.Request != nil && c.Request.URL != nil &&
		c.Request.URL.Path == "/v1/responses"
}

func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int, originalChannelRetry bool) bool {
	if openaiErr == nil {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	var requestContextErr error
	if c != nil && c.Request != nil {
		requestContextErr = c.Request.Context().Err()
	}
	if requestContextErr != nil || errors.Is(openaiErr, context.Canceled) {
		reason := "request_context_canceled"
		if errors.Is(requestContextErr, context.DeadlineExceeded) {
			reason = "request_context_deadline_exceeded"
		}
		if c != nil {
			logger.LogInfo(c, fmt.Sprintf("跳过重试：reason=%s status=%d code=%s", reason, openaiErr.StatusCode, openaiErr.GetErrorCode()))
		}
		return false
	}
	if originalChannelRetry && openaiErr.GetErrorCode() == types.ErrorCodeAuthUnavailable {
		if c != nil {
			logger.LogInfo(c, fmt.Sprintf("跳过重试：reason=auth_unavailable_same_channel status=%d code=%s", openaiErr.StatusCode, openaiErr.GetErrorCode()))
		}
		return false
	}
	if !originalChannelRetry && ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if !originalChannelRetry && c != nil {
		if _, ok := c.Get("specific_channel_id"); ok {
			return false
		}
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	if err == nil {
		return
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	if ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}
}

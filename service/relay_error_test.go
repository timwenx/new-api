package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryRelayErrorSpecificChannelSkipsChannelError(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("specific_channel_id", "1")
	err := types.NewError(errors.New("channel failed"), types.ErrorCodeChannelNoAvailableKey)

	require.False(t, ShouldRetryRelayError(c, err, 1, false))
	require.True(t, ShouldRetryRelayError(c, err, 1, true))
	require.False(t, ShouldRetryRelayError(c, err, 0, true))
}

func TestShouldRetryRelayErrorStopsWhenRequestContextIsCanceled(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)
	err := types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

	require.False(t, ShouldRetryRelayError(c, err, 5, true))
}

func TestShouldRetryRelayErrorStopsAuthUnavailableOnOriginalChannel(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "no auth available",
		Type:    "server_error",
		Code:    string(types.ErrorCodeAuthUnavailable),
	}, http.StatusServiceUnavailable)

	require.False(t, ShouldRetryRelayError(c, err, 5, true))
	require.True(t, ShouldRetryRelayError(c, err, 5, false))
}

func TestShouldRetryResponsesOnOriginalChannel(t *testing.T) {
	original := common.ResponsesSameChannelRetryEnabled
	t.Cleanup(func() {
		common.ResponsesSameChannelRetryEnabled = original
	})

	responsesContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	responsesContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.ResponsesSameChannelRetryEnabled = false
	require.False(t, ShouldRetryResponsesOnOriginalChannel(responsesContext))

	common.ResponsesSameChannelRetryEnabled = true
	require.True(t, ShouldRetryResponsesOnOriginalChannel(responsesContext))

	compactContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	compactContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	require.False(t, ShouldRetryResponsesOnOriginalChannel(compactContext))
}

func TestShouldRetryRelayErrorResponsesOriginalChannelOverridesAffinity(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(ginKeyChannelAffinitySkipRetry, true)
	err := types.NewErrorWithStatusCode(errors.New("upstream bad gateway"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	require.False(t, ShouldRetryRelayError(c, err, 1, false))
	require.True(t, ShouldRetryRelayError(c, err, 1, true))
	require.False(t, ShouldRetryRelayError(c, err, 0, true))
}

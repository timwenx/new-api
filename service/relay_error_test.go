package service

import (
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

package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/require"
)

func TestRetryParamOriginalChannelRetry(t *testing.T) {
	channel := &model.Channel{Id: 42}
	retryParam := &RetryParam{
		Retry:                new(int),
		OriginalChannelRetry: true,
	}

	retryParam.RememberOriginalChannel(channel)
	require.Nil(t, retryParam.OriginalChannelForRetry())

	retryParam.IncreaseRetry()
	require.Same(t, channel, retryParam.OriginalChannelForRetry())
}

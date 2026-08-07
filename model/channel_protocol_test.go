package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetChannelProtocolTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
		require.NoError(t, DB.Exec("DELETE FROM channels").Error)
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		InitChannelCache()
	})
}

func insertChannelProtocolCandidate(t *testing.T, id int, endpointType constant.EndpointType) {
	t.Helper()
	endpointValue := string(endpointType)
	priority := int64(0)
	weight := uint(0)
	channel := &Channel{
		Id:                 id,
		Type:               constant.ChannelTypeOpenAI,
		Key:                fmt.Sprintf("key-%d", id),
		Status:             common.ChannelStatusEnabled,
		Name:               fmt.Sprintf("channel-%d", id),
		Models:             "protocol-test-model",
		SupportedEndpoints: &endpointValue,
		Group:              "default",
		Priority:           &priority,
		Weight:             &weight,
	}
	require.NoError(t, channel.Insert())
}

func TestGetRandomSatisfiedChannelFiltersSupportedEndpoints(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			resetChannelProtocolTables(t)
			common.MemoryCacheEnabled = memoryCacheEnabled
			insertChannelProtocolCandidate(t, 501, constant.EndpointTypeOpenAI)
			insertChannelProtocolCandidate(t, 502, constant.EndpointTypeOpenAIResponse)
			if memoryCacheEnabled {
				InitChannelCache()
			}

			chatChannel, err := GetRandomSatisfiedChannel("default", "protocol-test-model", 0, "/v1/chat/completions")
			require.NoError(t, err)
			require.NotNil(t, chatChannel)
			assert.Equal(t, 501, chatChannel.Id)

			responsesChannel, err := GetRandomSatisfiedChannel("default", "protocol-test-model", 0, "/v1/responses")
			require.NoError(t, err)
			require.NotNil(t, responsesChannel)
			assert.Equal(t, 502, responsesChannel.Id)

			unsupportedChannel, err := GetRandomSatisfiedChannel("default", "protocol-test-model", 0, "/suno/submit/music")
			require.NoError(t, err)
			assert.Nil(t, unsupportedChannel)
		})
	}
}

func TestChannelSupportedEndpointsCompatibilityAndValidation(t *testing.T) {
	legacyChannel := &Channel{Type: constant.ChannelTypeOpenAI}
	assert.True(t, legacyChannel.SupportsRequestPath("/v1/responses", "gpt-5"))
	assert.True(t, legacyChannel.SupportsRequestPath("/suno/submit/music", "suno_music"))

	responsesOnly := string(constant.EndpointTypeOpenAIResponse)
	channel := &Channel{
		Type:               constant.ChannelTypeOpenAI,
		SupportedEndpoints: &responsesOnly,
	}
	assert.True(t, channel.SupportsRequestPath("/v1/responses", "gpt-5"))
	assert.False(t, channel.SupportsRequestPath("/v1/chat/completions", "gpt-5"))
	assert.NoError(t, channel.ValidateSupportedEndpoints())

	invalid := "openai,unknown"
	channel.SupportedEndpoints = &invalid
	assert.ErrorContains(t, channel.ValidateSupportedEndpoints(), "unknown")

	duplicate := "openai,openai"
	channel.SupportedEndpoints = &duplicate
	assert.ErrorContains(t, channel.ValidateSupportedEndpoints(), "duplicate")
}

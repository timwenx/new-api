package common

import (
	"bytes"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoTracksFastModeFromFinalUpstreamRequest(t *testing.T) {
	info := &RelayInfo{}

	filtered, err := RemoveDisabledFields(
		[]byte(`{"service_tier":"priority"}`),
		dto.ChannelOtherSettings{},
		false,
	)
	require.NoError(t, err)
	info.SetUpstreamFastModeFromRequestBody(filtered)
	assert.False(t, info.UpstreamFastMode)

	allowed, err := RemoveDisabledFields(
		[]byte(`{"service_tier":"priority"}`),
		dto.ChannelOtherSettings{AllowServiceTier: true},
		false,
	)
	require.NoError(t, err)
	info.SetUpstreamFastModeFromRequestBody(allowed)
	assert.True(t, info.UpstreamFastMode)

	info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"flex"}`))
	assert.False(t, info.UpstreamFastMode)

	requestBody := bytes.NewReader([]byte(`{"service_tier":"fast"}`))
	info.SetUpstreamFastModeFromRequestReader(requestBody)
	assert.True(t, info.UpstreamFastMode)
	position, err := requestBody.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Zero(t, position)
}

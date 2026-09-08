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

type dailyTokenMultiplierProbe struct {
	multiplier float64
}

func (p *dailyTokenMultiplierProbe) SetUsageMultiplier(multiplier float64) *types.NewAPIError {
	p.multiplier = multiplier
	return nil
}

func (p *dailyTokenMultiplierProbe) Settle(int) error { return nil }
func (p *dailyTokenMultiplierProbe) Refund() error    { return nil }

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
	probe := &dailyTokenMultiplierProbe{}
	info := &RelayInfo{TokenMultiplier: 2, DailyTokens: probe}

	filtered, err := RemoveDisabledFields(
		[]byte(`{"service_tier":"priority"}`),
		dto.ChannelOtherSettings{},
		false,
	)
	require.NoError(t, err)
	require.Nil(t, info.SetUpstreamFastModeFromRequestBody(filtered))
	assert.False(t, info.UpstreamFastMode)
	assert.Equal(t, 1.0, probe.multiplier)
	assert.Equal(t, 2.0, info.EffectiveTokenMultiplier())

	allowed, err := RemoveDisabledFields(
		[]byte(`{"service_tier":"priority"}`),
		dto.ChannelOtherSettings{AllowServiceTier: true},
		false,
	)
	require.NoError(t, err)
	require.Nil(t, info.SetUpstreamFastModeFromRequestBody(allowed))
	assert.True(t, info.UpstreamFastMode)
	assert.Equal(t, FastTokenMultiplier, probe.multiplier)
	assert.Equal(t, 3.0, info.EffectiveTokenMultiplier())

	require.Nil(t, info.SetUpstreamFastModeFromRequestBody([]byte(`{"service_tier":"flex"}`)))
	assert.False(t, info.UpstreamFastMode)
	assert.Equal(t, 1.0, probe.multiplier)

	requestBody := bytes.NewReader([]byte(`{"service_tier":"fast"}`))
	require.Nil(t, info.SetUpstreamFastModeFromRequestReader(requestBody))
	assert.True(t, info.UpstreamFastMode)
	assert.Equal(t, FastTokenMultiplier, probe.multiplier)
	position, err := requestBody.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Zero(t, position)
}

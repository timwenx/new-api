package common

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaleTokenCountAppliesMultiplierAndRounds(t *testing.T) {
	tokens, err := ScaleTokenCount(5_000, 2, 20_000)
	require.NoError(t, err)
	assert.EqualValues(t, 10_000, tokens)

	tokens, err = ScaleTokenCount(3, 1.5, 20_000)
	require.NoError(t, err)
	assert.EqualValues(t, 5, tokens)

	tokens, err = ScaleTokenCount(1, 0.1, 20_000)
	require.NoError(t, err)
	assert.EqualValues(t, 1, tokens)
}

func TestScaleTokenCountRejectsInvalidOrOverflowingValues(t *testing.T) {
	_, err := ScaleTokenCount(1, math.Inf(1), 10)
	require.Error(t, err)

	_, err = ScaleTokenCount(6, 2, 10)
	require.Error(t, err)
}

package common

import (
	"errors"
	"math"
)

// ScaleTokenCount applies a configured usage multiplier and rounds to the
// nearest whole token. The caller supplies the largest count its storage or
// accounting bucket can represent.
func ScaleTokenCount(tokens int64, multiplier float64, max int64) (int64, error) {
	if tokens < 0 {
		return 0, errors.New("token count cannot be negative")
	}
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, errors.New("token multiplier must be a positive finite number")
	}
	if max < 0 {
		return 0, errors.New("maximum token count cannot be negative")
	}

	scaled := math.Round(float64(tokens) * multiplier)
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) || scaled > float64(max) {
		return 0, errors.New("scaled token count exceeds the supported range")
	}
	if tokens > 0 && scaled < 1 {
		if max < 1 {
			return 0, errors.New("scaled token count exceeds the supported range")
		}
		return 1, nil
	}
	return int64(scaled), nil
}

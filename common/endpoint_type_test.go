package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEndpointTypeByPath(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		endpointType constant.EndpointType
	}{
		{name: "chat completions", path: "/v1/chat/completions", endpointType: constant.EndpointTypeOpenAI},
		{name: "responses", path: "/v1/responses", endpointType: constant.EndpointTypeOpenAIResponse},
		{name: "responses compact", path: "/v1/responses/compact", endpointType: constant.EndpointTypeOpenAIResponseCompact},
		{name: "anthropic messages", path: "/v1/messages", endpointType: constant.EndpointTypeAnthropic},
		{name: "gemini generate", path: "/v1beta/models/gemini-2.5-flash:generateContent", endpointType: constant.EndpointTypeGemini},
		{name: "gemini embedding", path: "/v1beta/models/text-embedding-004:embedContent", endpointType: constant.EndpointTypeEmbeddings},
		{name: "rerank", path: "/v1/rerank", endpointType: constant.EndpointTypeJinaRerank},
		{name: "image generation", path: "/v1/images/generations", endpointType: constant.EndpointTypeImageGeneration},
		{name: "video", path: "/v1/videos", endpointType: constant.EndpointTypeOpenAIVideo},
		{name: "other OpenAI endpoint", path: "/v1/audio/speech", endpointType: constant.EndpointTypeOpenAI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpointType, ok := GetEndpointTypeByPath(tt.path)

			require.True(t, ok)
			assert.Equal(t, tt.endpointType, endpointType)
		})
	}

	_, ok := GetEndpointTypeByPath("/suno/submit/music")
	assert.False(t, ok)
}

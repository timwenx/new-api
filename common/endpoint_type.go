package common

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// GetEndpointTypesByChannelType 获取渠道最优先端点类型（所有的渠道都支持 OpenAI 端点）
func GetEndpointTypesByChannelType(channelType int, modelName string) []constant.EndpointType {
	var endpointTypes []constant.EndpointType
	switch channelType {
	case constant.ChannelTypeJina:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeJinaRerank}
	//case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeMidjourney}
	//case constant.ChannelTypeSunoAPI:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeSuno}
	//case constant.ChannelTypeKling:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeKling}
	//case constant.ChannelTypeJimeng:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeJimeng}
	case constant.ChannelTypeAws:
		fallthrough
	case constant.ChannelTypeAnthropic:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeAnthropic, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeVertexAi:
		fallthrough
	case constant.ChannelTypeGemini:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeGemini, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeOpenRouter: // OpenRouter 只支持 OpenAI 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
	case constant.ChannelTypeXai:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse}
	case constant.ChannelTypeSora:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIVideo}
	default:
		if IsOpenAIResponseOnlyModel(modelName) {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIResponse}
		} else {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	}
	if IsImageGenerationModel(modelName) {
		// add to first
		endpointTypes = append([]constant.EndpointType{constant.EndpointTypeImageGeneration}, endpointTypes...)
	}
	return endpointTypes
}

// GetEndpointTypeByPath maps an incoming relay path to the endpoint protocol
// used by channel capability filtering.
func GetEndpointTypeByPath(requestPath string) (constant.EndpointType, bool) {
	switch {
	case requestPath == "/v1/responses":
		return constant.EndpointTypeOpenAIResponse, true
	case requestPath == "/v1/responses/compact":
		return constant.EndpointTypeOpenAIResponseCompact, true
	case requestPath == "/v1/messages":
		return constant.EndpointTypeAnthropic, true
	case requestPath == "/v1/rerank" || requestPath == "/rerank":
		return constant.EndpointTypeJinaRerank, true
	case requestPath == "/v1/embeddings" ||
		(strings.HasPrefix(requestPath, "/v1/engines/") && strings.HasSuffix(requestPath, "/embeddings")):
		return constant.EndpointTypeEmbeddings, true
	case requestPath == "/v1/edits" || strings.HasPrefix(requestPath, "/v1/images/"):
		return constant.EndpointTypeImageGeneration, true
	case requestPath == "/v1/videos" || strings.HasPrefix(requestPath, "/v1/videos/"):
		return constant.EndpointTypeOpenAIVideo, true
	case requestPath == "/pg/chat/completions":
		return constant.EndpointTypeOpenAI, true
	}

	if strings.HasPrefix(requestPath, "/v1beta/models/") || strings.HasPrefix(requestPath, "/v1/models/") {
		actionIndex := strings.LastIndex(requestPath, ":")
		if actionIndex < 0 {
			return "", false
		}
		switch requestPath[actionIndex+1:] {
		case "generateContent", "streamGenerateContent":
			return constant.EndpointTypeGemini, true
		case "embedContent", "batchEmbedContents":
			return constant.EndpointTypeEmbeddings, true
		default:
			return "", false
		}
	}

	if strings.HasPrefix(requestPath, "/v1/") {
		return constant.EndpointTypeOpenAI, true
	}
	return "", false
}

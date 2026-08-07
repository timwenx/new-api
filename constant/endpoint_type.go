package constant

type EndpointType string

const (
	EndpointTypeOpenAI                EndpointType = "openai"
	EndpointTypeOpenAIResponse        EndpointType = "openai-response"
	EndpointTypeOpenAIResponseCompact EndpointType = "openai-response-compact"
	EndpointTypeAnthropic             EndpointType = "anthropic"
	EndpointTypeGemini                EndpointType = "gemini"
	EndpointTypeJinaRerank            EndpointType = "jina-rerank"
	EndpointTypeImageGeneration       EndpointType = "image-generation"
	EndpointTypeEmbeddings            EndpointType = "embeddings"
	EndpointTypeOpenAIVideo           EndpointType = "openai-video"
	//EndpointTypeMidjourney     EndpointType = "midjourney-proxy"
	//EndpointTypeSuno           EndpointType = "suno-proxy"
	//EndpointTypeKling          EndpointType = "kling"
	//EndpointTypeJimeng         EndpointType = "jimeng"
)

func IsValidEndpointType(endpointType EndpointType) bool {
	switch endpointType {
	case EndpointTypeOpenAI,
		EndpointTypeOpenAIResponse,
		EndpointTypeOpenAIResponseCompact,
		EndpointTypeAnthropic,
		EndpointTypeGemini,
		EndpointTypeJinaRerank,
		EndpointTypeImageGeneration,
		EndpointTypeEmbeddings,
		EndpointTypeOpenAIVideo:
		return true
	default:
		return false
	}
}

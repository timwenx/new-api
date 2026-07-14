package relay

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	appmodel "github.com/QuantumNous/new-api/model"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareResponsesHTTPBridgeRequestExpandsLocalPreviousResponse(t *testing.T) {
	session := &responsesWSSession{}
	session.setResponsesHTTPBridgeContext("resp_local", []common.RawMessage{
		common.RawMessage(`{"role":"user","content":"first"}`),
		common.RawMessage(`{"type":"message","role":"assistant","content":[]}`),
	})

	req := dto.OpenAIResponsesRequest{
		Model:              "gpt-5.5",
		PreviousResponseID: "resp_local",
		Input:              common.RawMessage(`[{"type":"function_call_output","call_id":"call_1","output":"ok"}]`),
	}
	prepared, items, apiErr := session.prepareResponsesHTTPBridgeRequest(req)

	require.Nil(t, apiErr)
	assert.Empty(t, prepared.PreviousResponseID)
	require.Len(t, items, 3)
	var inputItems []common.RawMessage
	require.NoError(t, common.Unmarshal(prepared.Input, &inputItems))
	require.Len(t, inputItems, 3)
	assert.JSONEq(t, `{"role":"user","content":"first"}`, string(inputItems[0]))
	assert.JSONEq(t, `{"type":"function_call_output","call_id":"call_1","output":"ok"}`, string(inputItems[2]))
}

func TestPrepareResponsesHTTPBridgeRequestPreservesUnknownPreviousResponse(t *testing.T) {
	session := &responsesWSSession{}
	session.setResponsesHTTPBridgeContext("resp_local", nil)
	req := dto.OpenAIResponsesRequest{
		PreviousResponseID: "resp_stored_upstream",
		Input:              common.RawMessage(`"next"`),
	}

	prepared, items, apiErr := session.prepareResponsesHTTPBridgeRequest(req)

	require.Nil(t, apiErr)
	assert.Equal(t, "resp_stored_upstream", prepared.PreviousResponseID)
	assert.JSONEq(t, `"next"`, string(prepared.Input))
	require.Len(t, items, 1)
	assert.JSONEq(t, `{"role":"user","content":"next"}`, string(items[0]))
}

func TestResponsesHTTPBridgeWriterForwardsChunksAndHoldsTerminal(t *testing.T) {
	clientPeer, client, cleanup := newTestWebSocketPair(t)
	defer cleanup()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := newResponsesHTTPBridgeWriter(&responsesWSSession{client: client}, cancel)

	_, err := writer.WriteString("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n")
	require.NoError(t, err)
	_, err = writer.WriteString("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}]}}\n\n")
	require.NoError(t, err)
	require.NoError(t, writer.finalize())

	require.NoError(t, clientPeer.SetReadDeadline(time.Now().Add(time.Second)))
	messageType, message, err := clientPeer.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.JSONEq(t, `{"type":"response.output_text.delta","delta":"hi"}`, string(message))

	result := writer.snapshot()
	assert.Equal(t, "response.completed", result.terminalType)
	assert.Equal(t, "resp_1", result.responseID)
	require.Len(t, result.outputItems, 1)
	assert.JSONEq(t, `{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[]}]}}`, string(result.terminal))
}

func TestCommitResponsesHTTPBridgeContextUsesTerminalOutput(t *testing.T) {
	session := &responsesWSSession{}
	state := &responsesWSCallState{httpInputItems: []common.RawMessage{
		common.RawMessage(`{"role":"user","content":"hello"}`),
	}}
	session.commitResponsesHTTPBridgeContext(state, responsesHTTPBridgeResult{
		responseID: "resp_2",
		outputItems: []common.RawMessage{
			common.RawMessage(`{"type":"message","role":"assistant","content":[]}`),
		},
	})

	assert.Equal(t, "resp_2", session.httpResponseID)
	require.Len(t, session.httpContext, 2)
	assert.JSONEq(t, `{"type":"message","role":"assistant","content":[]}`, string(session.httpContext[1]))
}

func TestResponsesWSChannelSupportsAdvancedCustomResponsesRoute(t *testing.T) {
	channel := &appmodel.Channel{Type: appconstant.ChannelTypeAdvancedCustom}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/responses",
			Models:       []string{"gpt-5.5"},
		}},
	}})

	assert.True(t, responsesWSChannelSupportsRequest(channel, "/v1/responses", "gpt-5.5"))
	assert.False(t, responsesWSChannelSupportsRequest(channel, "/v1/responses", "gpt-5.6-sol"))
	assert.False(t, responsesWSChannelSupportsRequest(channel, "/v1/chat/completions", "gpt-5.5"))
}

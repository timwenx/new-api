package relay

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	appmodel "github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type responsesHTTPBridgeResult struct {
	terminal     []byte
	terminalType string
	responseID   string
	outputItems  []common.RawMessage
}

type responsesHTTPBridgeWriter struct {
	mu      sync.Mutex
	session *responsesWSSession
	cancel  context.CancelFunc
	header  http.Header
	status  int
	size    int
	buffer  bytes.Buffer
	result  responsesHTTPBridgeResult
	err     error
}

var _ gin.ResponseWriter = (*responsesHTTPBridgeWriter)(nil)

func newResponsesHTTPBridgeWriter(session *responsesWSSession, cancel context.CancelFunc) *responsesHTTPBridgeWriter {
	return &responsesHTTPBridgeWriter{
		session: session,
		cancel:  cancel,
		header:  make(http.Header),
		status:  http.StatusOK,
		size:    -1,
	}
}

func (w *responsesHTTPBridgeWriter) Header() http.Header {
	return w.header
}

func (w *responsesHTTPBridgeWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size >= 0 || code <= 0 {
		return
	}
	w.status = code
}

func (w *responsesHTTPBridgeWriter) WriteHeaderNow() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeHeaderNowLocked()
}

func (w *responsesHTTPBridgeWriter) writeHeaderNowLocked() {
	if w.size < 0 {
		w.size = 0
	}
}

func (w *responsesHTTPBridgeWriter) Status() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *responsesHTTPBridgeWriter) Size() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

func (w *responsesHTTPBridgeWriter) Written() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size >= 0
}

func (w *responsesHTTPBridgeWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *responsesHTTPBridgeWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	w.writeHeaderNowLocked()
	w.size += len(data)
	_, _ = w.buffer.Write(data)
	if err := w.flushEventsLocked(false); err != nil {
		w.failLocked(err)
		return 0, err
	}
	return len(data), nil
}

func (w *responsesHTTPBridgeWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeHeaderNowLocked()
	if w.err != nil {
		return
	}
	if err := w.flushEventsLocked(false); err != nil {
		w.failLocked(err)
	}
}

func (w *responsesHTTPBridgeWriter) CloseNotify() <-chan bool {
	return make(chan bool)
}

func (w *responsesHTTPBridgeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("responses HTTP bridge does not support hijacking")
}

func (w *responsesHTTPBridgeWriter) Pusher() http.Pusher {
	return nil
}

func (w *responsesHTTPBridgeWriter) finalize() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	if err := w.flushEventsLocked(true); err != nil {
		w.failLocked(err)
	}
	return w.err
}

func (w *responsesHTTPBridgeWriter) snapshot() responsesHTTPBridgeResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	return responsesHTTPBridgeResult{
		terminal:     append([]byte(nil), w.result.terminal...),
		terminalType: w.result.terminalType,
		responseID:   w.result.responseID,
		outputItems:  cloneResponsesRawMessages(w.result.outputItems),
	}
}

func (w *responsesHTTPBridgeWriter) failLocked(err error) {
	if err == nil || w.err != nil {
		return
	}
	w.err = err
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *responsesHTTPBridgeWriter) flushEventsLocked(final bool) error {
	for {
		data := w.buffer.Bytes()
		idx, separatorSize := responsesSSESeparator(data)
		if idx < 0 {
			if !final || len(bytes.TrimSpace(data)) == 0 {
				return nil
			}
			idx = len(data)
			separatorSize = 0
		}

		block := append([]byte(nil), data[:idx]...)
		w.buffer.Next(idx + separatorSize)
		if err := w.handleEventBlockLocked(block); err != nil {
			return err
		}
	}
}

func responsesSSESeparator(data []byte) (int, int) {
	lf := bytes.Index(data, []byte("\n\n"))
	crlf := bytes.Index(data, []byte("\r\n\r\n"))
	switch {
	case lf < 0:
		if crlf < 0 {
			return -1, 0
		}
		return crlf, 4
	case crlf < 0 || lf < crlf:
		return lf, 2
	default:
		return crlf, 4
	}
}

func (w *responsesHTTPBridgeWriter) handleEventBlockLocked(block []byte) error {
	block = bytes.ReplaceAll(block, []byte("\r\n"), []byte("\n"))
	lines := bytes.Split(block, []byte("\n"))
	dataLines := make([]string, 0, 1)
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == ':' || bytes.HasPrefix(line, []byte("event:")) {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		dataLines = append(dataLines, strings.TrimSpace(string(line[len("data:"):])))
	}
	if len(dataLines) == 0 {
		return nil
	}
	payload := []byte(strings.Join(dataLines, "\n"))
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return nil
	}

	var event struct {
		Type     string            `json:"type"`
		Response common.RawMessage `json:"response"`
		Item     common.RawMessage `json:"item"`
	}
	if err := common.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("invalid responses SSE event: %w", err)
	}
	w.observeEventLocked(event.Type, event.Response, event.Item)
	if isResponsesHTTPTerminalEvent(event.Type) {
		w.result.terminal = append(w.result.terminal[:0], payload...)
		w.result.terminalType = event.Type
		return nil
	}
	return w.session.writeClient(websocket.TextMessage, payload)
}

func (w *responsesHTTPBridgeWriter) observeEventLocked(eventType string, responseRaw common.RawMessage, itemRaw common.RawMessage) {
	if eventType == dto.ResponsesOutputTypeItemDone && len(itemRaw) > 0 && !bytes.Equal(bytes.TrimSpace(itemRaw), []byte("null")) {
		w.result.outputItems = append(w.result.outputItems, append(common.RawMessage(nil), itemRaw...))
	}
	if len(responseRaw) == 0 || bytes.Equal(bytes.TrimSpace(responseRaw), []byte("null")) {
		return
	}
	var response struct {
		ID     string              `json:"id"`
		Output []common.RawMessage `json:"output"`
	}
	if err := common.Unmarshal(responseRaw, &response); err != nil {
		return
	}
	if response.ID != "" {
		w.result.responseID = response.ID
	}
	if len(response.Output) > 0 {
		w.result.outputItems = cloneResponsesRawMessages(response.Output)
	}
}

func isResponsesHTTPTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.incomplete", "response.failed", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
	}
}

func isResponsesHTTPSuccessTerminal(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.incomplete":
		return true
	default:
		return false
	}
}

func (s *responsesWSSession) usesHTTPBridge() bool {
	s.targetWriteMu.Lock()
	defer s.targetWriteMu.Unlock()
	return s.httpBridge
}

func (s *responsesWSSession) startResponsesHTTPBridge(create responsesWSCreateRequest, eventID string, commitRate middleware.ModelRequestRateLimitCommit, channel *appmodel.Channel, firstConnection bool) *types.NewAPIError {
	if channel == nil {
		commitRate(false)
		return types.NewError(errors.New("responses HTTP bridge channel is missing"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	s.httpLifecycleMu.Lock()
	defer s.httpLifecycleMu.Unlock()

	generateFalse, err := responsesWSGenerateFalse(create.Generate)
	if err != nil {
		commitRate(false)
		return newResponsesWSInvalidRequestError(err)
	}
	if generateFalse {
		inputItems, apiErr := responsesHTTPInputItems(create.Request.Input)
		if apiErr != nil {
			commitRate(false)
			return apiErr
		}
		if firstConnection {
			s.activateResponsesHTTPBridge(channel, create.Request.Model)
		}
		responseID := "resp_newapi_warm_" + common.GetUUID()
		s.setResponsesHTTPBridgeContext(responseID, inputItems)
		if err := s.sendResponsesHTTPWarmup(create.Request.Model, responseID); err != nil {
			commitRate(false)
			return types.NewError(err, types.ErrorCodeBadResponse)
		}
		commitRate(true)
		return nil
	}

	state, _, apiErr := s.prepareCall(create, commitRate, false)
	if apiErr != nil {
		commitRate(false)
		return apiErr
	}
	if !s.tryReserveCurrent(state) {
		state.refund(s.c)
		commitRate(false)
		return types.NewErrorWithStatusCode(
			errors.New("another response.create is already in progress on this websocket connection"),
			types.ErrorCodeInvalidRequest,
			http.StatusConflict,
			types.ErrOptionWithSkipRetry(),
		)
	}

	bridgeContext, cancel, err := s.newResponsesHTTPBridgeContext(state)
	if err != nil {
		s.finishResponsesHTTPBridgeCall(state, false, true)
		return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if firstConnection {
		s.activateResponsesHTTPBridge(channel, create.Request.Model)
	}
	s.targetWriteMu.Lock()
	s.httpCancel = cancel
	s.targetWriteMu.Unlock()

	s.httpWG.Add(1)
	go func() {
		defer s.httpWG.Done()
		defer cancel()
		defer common.CleanupBodyStorage(bridgeContext)

		writer := newResponsesHTTPBridgeWriter(s, cancel)
		bridgeContext.Writer = writer
		relayErr := ResponsesHelper(bridgeContext, state.info)
		writerErr := writer.finalize()
		if relayErr != nil {
			s.finishResponsesHTTPBridgeCall(state, false, true)
			relayErr, _ = s.processChannelError(channel, relayErr, nil)
			s.sendError(eventID, relayErr)
			return
		}

		result := writer.snapshot()
		if isResponsesHTTPSuccessTerminal(result.terminalType) {
			s.commitResponsesHTTPBridgeContext(state, result)
			service.RecordChannelAffinity(s.c, channel.Id)
		}
		success := isResponsesHTTPSuccessTerminal(result.terminalType)
		s.finishResponsesHTTPBridgeCall(state, success, false)
		if writerErr != nil {
			return
		}
		if len(result.terminal) == 0 {
			s.sendError(eventID, types.NewError(errors.New("upstream HTTP responses stream ended without a terminal event"), types.ErrorCodeBadResponse))
			return
		}
		if err := s.writeClient(websocket.TextMessage, result.terminal); err != nil {
			return
		}
	}()

	return nil
}

func (s *responsesWSSession) activateResponsesHTTPBridge(channel *appmodel.Channel, modelName string) {
	s.targetWriteMu.Lock()
	s.httpBridge = true
	s.targetWriteMu.Unlock()
	s.lockedModel = modelName
	s.lockedChannel = channel
	s.registerChannelClose(channel.Id, modelName, "http")
}

func (s *responsesWSSession) finishResponsesHTTPBridgeCall(state *responsesWSCallState, success bool, refund bool) {
	if state == nil || !s.clearCurrent(state) {
		return
	}
	if refund {
		state.refund(s.c)
	}
	if state.commitRate != nil {
		state.commitRate(success)
	}
}

func (s *responsesWSSession) newResponsesHTTPBridgeContext(state *responsesWSCallState) (*gin.Context, context.CancelFunc, error) {
	if state == nil || state.info == nil {
		return nil, nil, errors.New("responses HTTP bridge state is missing")
	}
	request, ok := state.info.Request.(*dto.OpenAIResponsesRequest)
	if !ok || request == nil {
		return nil, nil, fmt.Errorf("invalid responses HTTP bridge request type %T", state.info.Request)
	}
	body, err := common.Marshal(request)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal responses HTTP bridge request: %w", err)
	}

	requestContext, cancel := context.WithCancel(s.c.Request.Context())
	httpRequest := s.c.Request.Clone(requestContext)
	httpRequest.Method = http.MethodPost
	httpRequest.URL.Path = "/v1/responses"
	httpRequest.URL.RawPath = ""
	httpRequest.RequestURI = "/v1/responses"
	httpRequest.Header = httpRequest.Header.Clone()
	removeResponsesWebSocketHeaders(httpRequest.Header)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Body = io.NopCloser(bytes.NewReader(body))
	httpRequest.ContentLength = int64(len(body))

	bridgeContext := s.c.Copy()
	bridgeContext.Request = httpRequest
	bridgeContext.Set(common.KeyBodyStorage, nil)
	bridgeContext.Set(common.KeyRequestBody, body)
	return bridgeContext, cancel, nil
}

func removeResponsesWebSocketHeaders(header http.Header) {
	for key := range header {
		lower := strings.ToLower(key)
		if lower == "upgrade" || lower == "connection" || strings.HasPrefix(lower, "sec-websocket-") {
			header.Del(key)
		}
	}
	header.Del("Content-Length")
}

func (s *responsesWSSession) prepareResponsesHTTPBridgeRequest(req dto.OpenAIResponsesRequest) (dto.OpenAIResponsesRequest, []common.RawMessage, *types.NewAPIError) {
	inputItems, apiErr := responsesHTTPInputItems(req.Input)
	if apiErr != nil {
		return req, nil, apiErr
	}

	s.httpContextMu.Lock()
	localResponseID := s.httpResponseID
	localContext := cloneResponsesRawMessages(s.httpContext)
	s.httpContextMu.Unlock()

	if req.PreviousResponseID != "" && req.PreviousResponseID == localResponseID {
		inputItems = append(localContext, inputItems...)
		input, err := common.Marshal(inputItems)
		if err != nil {
			return req, nil, types.NewError(fmt.Errorf("marshal responses HTTP bridge context: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		req.Input = input
		req.PreviousResponseID = ""
	}
	return req, inputItems, nil
}

func responsesHTTPInputItems(input common.RawMessage) ([]common.RawMessage, *types.NewAPIError) {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	switch trimmed[0] {
	case '[':
		var items []common.RawMessage
		if err := common.Unmarshal(trimmed, &items); err != nil {
			return nil, newResponsesWSInvalidRequestError(fmt.Errorf("invalid responses input array: %w", err))
		}
		return cloneResponsesRawMessages(items), nil
	case '{':
		var object map[string]any
		if err := common.Unmarshal(trimmed, &object); err != nil {
			return nil, newResponsesWSInvalidRequestError(fmt.Errorf("invalid responses input object: %w", err))
		}
		return []common.RawMessage{append(common.RawMessage(nil), trimmed...)}, nil
	case '"':
		var text string
		if err := common.Unmarshal(trimmed, &text); err != nil {
			return nil, newResponsesWSInvalidRequestError(fmt.Errorf("invalid responses text input: %w", err))
		}
		item, err := common.Marshal(map[string]any{"role": "user", "content": text})
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		return []common.RawMessage{item}, nil
	default:
		return nil, newResponsesWSInvalidRequestError(errors.New("responses input must be a string, object, or array"))
	}
}

func cloneResponsesRawMessages(items []common.RawMessage) []common.RawMessage {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]common.RawMessage, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, append(common.RawMessage(nil), item...))
	}
	return cloned
}

func (s *responsesWSSession) commitResponsesHTTPBridgeContext(state *responsesWSCallState, result responsesHTTPBridgeResult) {
	if state == nil || result.responseID == "" {
		return
	}
	contextItems := cloneResponsesRawMessages(state.httpInputItems)
	contextItems = append(contextItems, cloneResponsesRawMessages(result.outputItems)...)
	s.setResponsesHTTPBridgeContext(result.responseID, contextItems)
}

func (s *responsesWSSession) setResponsesHTTPBridgeContext(responseID string, items []common.RawMessage) {
	s.httpContextMu.Lock()
	s.httpResponseID = responseID
	s.httpContext = cloneResponsesRawMessages(items)
	s.httpContextMu.Unlock()
}

func (s *responsesWSSession) clearResponsesHTTPBridgeContext() {
	s.setResponsesHTTPBridgeContext("", nil)
}

func responsesWSGenerateFalse(raw common.RawMessage) (bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, nil
	}
	var generate bool
	if err := common.Unmarshal(raw, &generate); err != nil {
		return false, fmt.Errorf("generate must be a boolean: %w", err)
	}
	return !generate, nil
}

func (s *responsesWSSession) sendResponsesHTTPWarmup(modelName string, responseID string) error {
	createdAt := time.Now().Unix()
	created, err := common.Marshal(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"created_at": createdAt,
			"status":     "in_progress",
			"model":      modelName,
			"output":     []any{},
		},
	})
	if err != nil {
		return err
	}
	completed, err := common.Marshal(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"created_at": createdAt,
			"status":     "completed",
			"model":      modelName,
			"output":     []any{},
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		},
	})
	if err != nil {
		return err
	}
	if err := s.writeClient(websocket.TextMessage, created); err != nil {
		return err
	}
	return s.writeClient(websocket.TextMessage, completed)
}

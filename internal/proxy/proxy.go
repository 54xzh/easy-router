package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"easy-router/internal/store"
)

type Handler struct {
	store  *store.Store
	client *http.Client
}

const statusClientClosedRequest = 499

type candidate struct {
	Provider           store.Provider
	Model              store.Model
	Endpoint           string
	Conversion         conversionMode
	StreamIncludeUsage bool
	RequestBody        []byte
	AttemptLabel       string
}

type conversionMode string

const (
	conversionNone            conversionMode = ""
	conversionResponsesToChat conversionMode = "responses_to_chat"
	conversionChatToResponses conversionMode = "chat_to_responses"
)

func Register(mux *http.ServeMux, db *store.Store) {
	h := &Handler{
		store: db,
		client: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
	mux.HandleFunc("/v1/models", h.withProxyAuth(h.models))
	mux.HandleFunc("/v1/chat/completions", h.withProxyAuth(func(w http.ResponseWriter, r *http.Request) {
		h.completion(w, r, "chat")
	}))
	mux.HandleFunc("/v1/responses", h.withProxyAuth(func(w http.ResponseWriter, r *http.Request) {
		h.completion(w, r, "responses")
	}))
}

func (h *Handler) withProxyAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.store.ValidateProxyKey(bearerToken(r.Header.Get("Authorization"))) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "缺少或无效的代理访问密钥"})
			return
		}
		next(w, r)
	}
}

func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	routes, err := h.store.EnabledRoutes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	data := make([]map[string]any, 0, len(routes))
	for _, route := range routes {
		data = append(data, modelObject(route.ID, "easy-router"))
	}
	rawModels, err := h.store.RawModelsForModelsEndpoint()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	for _, model := range rawModels {
		data = append(data, modelObject(model.InternalID, model.ProviderID))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}

func modelObject(id, owner string) map[string]any {
	return map[string]any{
		"id":       id,
		"object":   "model",
		"created":  0,
		"owned_by": owner,
	}
}

func (h *Handler) completion(w http.ResponseWriter, r *http.Request, api string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	start := time.Now()
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "读取请求失败"})
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求不是有效 JSON"})
		return
	}
	clientModel, _ := payload["model"].(string)
	if clientModel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少 model"})
		return
	}
	streaming := payload["stream"] == true
	attempts := []store.AttemptLog{}
	requestLog := store.RequestLog{
		ID:          newRequestID(),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		API:         api,
		RouteID:     clientModel,
		ClientModel: clientModel,
		Status:      "error",
	}

	autoDisableEnabled, err := h.store.AutoDisableModelsEnabled()
	if err != nil {
		requestLog.HTTPStatus = http.StatusInternalServerError
		requestLog.DurationMS = time.Since(start).Milliseconds()
		requestLog.Error = err.Error()
		_ = h.store.AddRequestLog(requestLog)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	upstreamTimeout, err := h.store.UpstreamTimeout()
	if err != nil {
		requestLog.HTTPStatus = http.StatusBadRequest
		requestLog.DurationMS = time.Since(start).Milliseconds()
		requestLog.Error = err.Error()
		_ = h.store.AddRequestLog(requestLog)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	candidates, routeID, err := h.resolveCandidates(clientModel, payload, api, streaming, autoDisableEnabled)
	requestLog.RouteID = routeID
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRouteNotFound) || errors.Is(err, errNoCandidate) {
			status = http.StatusBadGateway
		}
		requestLog.HTTPStatus = status
		requestLog.DurationMS = time.Since(start).Milliseconds()
		requestLog.Error = err.Error()
		_ = h.store.AddRequestLog(requestLog)
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}

	for idx, item := range candidates {
		if requestCanceled(r.Context()) {
			requestLog.Status = "canceled"
			requestLog.Attempts = attempts
			requestLog.HTTPStatus = statusClientClosedRequest
			requestLog.DurationMS = time.Since(start).Milliseconds()
			requestLog.Error = downstreamCancelMessage(r.Context().Err())
			_ = h.store.AddRequestLog(requestLog)
			writeJSON(w, statusClientClosedRequest, map[string]any{"error": requestLog.Error})
			return
		}
		attemptStart := time.Now()
		statusCode, responseBody, responseHeaders, copyErr, fallback, err := h.callUpstream(w, r, item, streaming, upstreamTimeout)
		duration := time.Since(attemptStart).Milliseconds()
		attempt := store.AttemptLog{
			Position:   idx + 1,
			ModelID:    item.Model.InternalID,
			ProviderID: item.Model.ProviderID,
			HTTPStatus: statusCode,
			DurationMS: duration,
		}

		if err != nil {
			if requestCanceled(r.Context()) {
				attempt.Status = "canceled"
				attempt.HTTPStatus = statusClientClosedRequest
				attempt.Error = downstreamCancelMessage(r.Context().Err())
				attempts = append(attempts, attempt)
				requestLog.Status = "canceled"
				requestLog.Attempts = attempts
				requestLog.FinalModel = item.Model.InternalID
				requestLog.HTTPStatus = statusClientClosedRequest
				requestLog.DurationMS = time.Since(start).Milliseconds()
				requestLog.Error = attempt.Error
				_ = h.store.AddRequestLog(requestLog)
				writeJSON(w, statusClientClosedRequest, map[string]any{"error": requestLog.Error})
				return
			}
			attempt.Status = "failed"
			attempt.Error = err.Error()
			attempts = append(attempts, attempt)
			if autoDisableEnabled {
				_ = h.store.RecordModelFailure(item.Model.InternalID, err.Error())
			}
			if fallback {
				continue
			}
			requestLog.Attempts = attempts
			requestLog.FinalModel = item.Model.InternalID
			requestLog.HTTPStatus = statusCode
			requestLog.DurationMS = time.Since(start).Milliseconds()
			requestLog.Error = err.Error()
			_ = h.store.AddRequestLog(requestLog)
			writeJSON(w, statusCode, map[string]any{"error": err.Error()})
			return
		}

		attempt.Status = "success"
		attempts = append(attempts, attempt)
		_ = h.store.RecordModelSuccess(item.Model.InternalID)
		requestLog.Status = "success"
		requestLog.Attempts = attempts
		requestLog.FinalModel = item.Model.InternalID
		requestLog.HTTPStatus = statusCode
		requestLog.DurationMS = time.Since(start).Milliseconds()

		if streaming {
			if copyErr != nil {
				requestLog.Status = "stream_error"
				requestLog.Error = copyErr.Error()
			}
			_ = h.store.AddRequestLog(requestLog)
			return
		}

		usage := parseUsage(responseBody)
		requestLog.PromptTokens = usage.prompt
		requestLog.CompletionTokens = usage.completion
		requestLog.TotalTokens = usage.total
		_ = h.store.AddRequestLog(requestLog)
		copyHeaders(w.Header(), responseHeaders)
		if item.Conversion != conversionNone {
			w.Header().Del("Content-Length")
			w.Header().Del("Content-Encoding")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write(responseBody)
		return
	}

	requestLog.Attempts = attempts
	requestLog.HTTPStatus = http.StatusBadGateway
	requestLog.DurationMS = time.Since(start).Milliseconds()
	requestLog.Error = "路由内所有可用模型都失败"
	_ = h.store.AddRequestLog(requestLog)
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": requestLog.Error})
}

func (h *Handler) callUpstream(w http.ResponseWriter, r *http.Request, item candidate, streaming bool, upstreamTimeout time.Duration) (int, []byte, http.Header, error, bool, error) {
	endpoint := joinEndpoint(item.Provider.BaseURL, item.Endpoint)
	ctx := r.Context()
	var cancel context.CancelFunc
	if !streaming && upstreamTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, upstreamTimeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(item.RequestBody))
	if err != nil {
		return http.StatusBadGateway, nil, nil, nil, true, err
	}
	copyRequestHeaders(req.Header, r.Header)
	req.Header.Set("Content-Type", "application/json")
	if item.Provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+item.Provider.APIKey)
	}
	for key, value := range item.Provider.ExtraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		if requestTimedOut(ctx, r.Context()) {
			return http.StatusGatewayTimeout, nil, nil, nil, true, fmt.Errorf("上游请求超过 %s", upstreamTimeout)
		}
		if errors.Is(err, context.Canceled) {
			return http.StatusBadGateway, nil, nil, nil, true, fmt.Errorf("上游连接取消：%w", err)
		}
		return http.StatusBadGateway, nil, nil, nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		fallback := isFallbackable(resp.StatusCode, item.Endpoint)
		message := fmt.Sprintf("上游返回 %d：%s", resp.StatusCode, strings.TrimSpace(string(payload)))
		return resp.StatusCode, payload, resp.Header.Clone(), nil, fallback, errors.New(message)
	}

	if streaming {
		copyHeaders(w.Header(), resp.Header)
		if item.Conversion != conversionNone {
			w.Header().Del("Content-Length")
			w.Header().Del("Content-Encoding")
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(resp.StatusCode)
			copyErr := convertStream(w, resp.Body, item.Conversion, item.Model.OriginalID, item.StreamIncludeUsage)
			return resp.StatusCode, nil, resp.Header.Clone(), copyErr, false, nil
		}
		w.WriteHeader(resp.StatusCode)
		_, copyErr := io.Copy(w, resp.Body)
		return resp.StatusCode, nil, resp.Header.Clone(), copyErr, false, nil
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		if requestTimedOut(ctx, r.Context()) {
			return http.StatusGatewayTimeout, nil, nil, nil, true, fmt.Errorf("上游请求超过 %s", upstreamTimeout)
		}
		return http.StatusBadGateway, nil, nil, nil, true, err
	}
	if item.Conversion != conversionNone {
		payload, err = convertResponsePayload(payload, item.Conversion)
		if err != nil {
			return http.StatusBadGateway, nil, resp.Header.Clone(), nil, true, err
		}
	}
	return resp.StatusCode, payload, resp.Header.Clone(), nil, false, nil
}

func (h *Handler) resolveCandidates(clientModel string, payload map[string]any, api string, streaming bool, autoDisableEnabled bool) ([]candidate, string, error) {
	if route, err := h.store.GetRoute(clientModel); err == nil {
		if !route.Enabled {
			return nil, route.ID, errors.New("路由模型已禁用")
		}
		items, err := h.routeCandidates(route, payload, api, streaming, autoDisableEnabled)
		return items, route.ID, err
	}

	raw, err := h.rawCandidate(clientModel, payload, api, streaming, autoDisableEnabled)
	if err != nil {
		return nil, clientModel, err
	}
	return []candidate{raw}, clientModel, nil
}

func (h *Handler) rawCandidate(modelID string, payload map[string]any, api string, streaming bool, autoDisableEnabled bool) (candidate, error) {
	rawModels, err := h.store.RawModelsForModelsEndpoint()
	if err != nil {
		return candidate{}, err
	}
	for _, model := range rawModels {
		if model.InternalID == modelID {
			item, ok, err := h.maybeCandidate(model, payload, api, streaming, autoDisableEnabled)
			if err != nil {
				return candidate{}, err
			}
			if !ok {
				return candidate{}, errNoCandidate
			}
			return item, nil
		}
	}
	return candidate{}, errRouteNotFound
}

func (h *Handler) routeCandidates(route store.Route, payload map[string]any, api string, streaming bool, autoDisableEnabled bool) ([]candidate, error) {
	overrides := map[string]bool{}
	for _, override := range route.Overrides {
		overrides[override.TargetType+"|"+override.TargetID] = override.Disabled
	}
	var out []candidate
	conversionErrors := []string{}
	for _, step := range route.Steps {
		if !step.Enabled || overrides[step.TargetType+"|"+step.TargetID] {
			continue
		}
		if step.TargetType == "model" {
			if overrides["model|"+step.TargetID] {
				continue
			}
			model, err := h.store.GetModel(step.TargetID)
			if err == nil {
				if item, ok, err := h.maybeCandidate(model, payload, api, streaming, autoDisableEnabled); err != nil {
					if errors.Is(err, errConversionUnsupported) {
						conversionErrors = append(conversionErrors, model.InternalID+" "+err.Error())
						continue
					}
					return nil, err
				} else if ok {
					out = append(out, item)
				}
			}
			continue
		}
		if step.TargetType == "group" {
			group, err := h.store.GetGroup(step.TargetID)
			if err != nil || !group.Enabled {
				continue
			}
			models := make([]store.Model, 0, len(group.Members))
			for _, member := range group.Members {
				if !member.Enabled || overrides["model|"+member.ModelID] {
					continue
				}
				model, err := h.store.GetModel(member.ModelID)
				if err == nil {
					models = append(models, model)
				}
			}
			models = h.orderGroupModels(group, models)
			for _, model := range models {
				if item, ok, err := h.maybeCandidate(model, payload, api, streaming, autoDisableEnabled); err != nil {
					if errors.Is(err, errConversionUnsupported) {
						conversionErrors = append(conversionErrors, model.InternalID+" "+err.Error())
						continue
					}
					return nil, err
				} else if ok {
					out = append(out, item)
				}
			}
		}
	}
	if len(out) == 0 {
		if len(conversionErrors) > 0 {
			return nil, fmt.Errorf("%w：%s", errNoCandidate, strings.Join(conversionErrors, "；"))
		}
		return nil, errNoCandidate
	}
	return out, nil
}

func (h *Handler) orderGroupModels(group store.ModelGroup, models []store.Model) []store.Model {
	if len(models) <= 1 {
		return models
	}
	ordered := append([]store.Model(nil), models...)
	switch group.Strategy {
	case "random":
		rand.Shuffle(len(ordered), func(i, j int) {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		})
	case "round_robin":
		start, err := h.store.AdvanceGroupCursor(group.ID, len(ordered))
		if err == nil && start > 0 {
			ordered = append(ordered[start:], ordered[:start]...)
		}
	}
	return ordered
}

func (h *Handler) maybeCandidate(model store.Model, payload map[string]any, api string, streaming bool, autoDisableEnabled bool) (candidate, bool, error) {
	if !model.Enabled || !model.ProviderEnabled || (autoDisableEnabled && (model.AutoDisabled || model.CoolingDown())) {
		return candidate{}, false, nil
	}
	if streaming && !model.SupportsStream {
		return candidate{}, false, nil
	}
	if api == "chat" && !model.SupportsChat && !model.SupportsResponses {
		return candidate{}, false, nil
	}
	if api == "responses" && !model.SupportsResponses && !model.SupportsChat {
		return candidate{}, false, nil
	}
	return h.buildCandidate(model, payload, api, streaming)
}

func (h *Handler) buildCandidate(model store.Model, payload map[string]any, api string, streaming bool) (candidate, bool, error) {
	provider, _, err := h.store.LoadProviderForModel(model.InternalID)
	if err != nil {
		return candidate{}, false, err
	}
	if !provider.Enabled {
		return candidate{}, false, nil
	}
	nextPayload := cloneMap(payload)
	nextPayload["model"] = model.OriginalID
	endpoint := "/chat/completions"
	conversion := conversionNone
	streamIncludeUsage := false
	if api == "chat" {
		if model.SupportsChat {
			endpoint = "/chat/completions"
		} else {
			endpoint = "/responses"
			streamIncludeUsage = chatStreamIncludeUsage(nextPayload)
			nextPayload, err = convertChatToResponses(nextPayload)
			if err != nil {
				return candidate{}, false, fmt.Errorf("%w：%v", errConversionUnsupported, err)
			}
			conversion = conversionChatToResponses
		}
	} else if api == "responses" {
		if model.SupportsResponses {
			endpoint = "/responses"
		} else {
			nextPayload, err = convertResponsesToChat(nextPayload)
			if err != nil {
				return candidate{}, false, fmt.Errorf("%w：%v", errConversionUnsupported, err)
			}
			conversion = conversionResponsesToChat
		}
	}
	body, err := json.Marshal(nextPayload)
	if err != nil {
		return candidate{}, false, err
	}
	return candidate{
		Provider:           provider,
		Model:              model,
		Endpoint:           endpoint,
		Conversion:         conversion,
		StreamIncludeUsage: streamIncludeUsage,
		RequestBody:        body,
		AttemptLabel:       model.InternalID,
	}, true, nil
}

func convertResponsesToChat(input map[string]any) (map[string]any, error) {
	allowed := map[string]bool{
		"model": true, "input": true, "instructions": true, "stream": true,
		"temperature": true, "top_p": true, "max_output_tokens": true, "max_tokens": true,
		"stop": true, "user": true, "metadata": true, "presence_penalty": true, "frequency_penalty": true,
		"tools": true, "tool_choice": true, "parallel_tool_calls": true,
		"reasoning": true,
	}
	for key := range input {
		if !allowed[key] {
			return nil, fmt.Errorf("当前上游不支持 Responses 字段 %q，且无法安全转换", key)
		}
	}
	out := map[string]any{}
	for _, key := range []string{"model", "stream", "temperature", "top_p", "stop", "user", "presence_penalty", "frequency_penalty"} {
		if value, ok := input[key]; ok {
			out[key] = value
		}
	}
	if value, ok := input["max_output_tokens"]; ok {
		out["max_tokens"] = value
	} else if value, ok := input["max_tokens"]; ok {
		out["max_tokens"] = value
	}
	if tools, ok := input["tools"]; ok {
		converted, err := responsesToolsToChat(tools)
		if err != nil {
			return nil, err
		}
		out["tools"] = converted
	}
	if choice, ok := input["tool_choice"]; ok {
		converted, err := responsesToolChoiceToChat(choice)
		if err != nil {
			return nil, err
		}
		out["tool_choice"] = converted
	}
	if value, ok := input["parallel_tool_calls"]; ok {
		out["parallel_tool_calls"] = value
	}
	if reasoning, ok := input["reasoning"]; ok {
		if effort, ok := reasoningEffort(reasoning); ok {
			out["reasoning_effort"] = effort
		}
	}
	messages := []map[string]any{}
	if instructions, ok := input["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	rawInput, ok := input["input"]
	if !ok {
		return nil, errors.New("Responses 请求缺少 input，无法转换")
	}
	switch value := rawInput.(type) {
	case string:
		messages = append(messages, map[string]any{"role": "user", "content": value})
	case []any:
		for _, item := range value {
			message, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("Responses input 只支持字符串或 message/item 数组")
			}
			itemType, _ := message["type"].(string)
			switch itemType {
			case "", "message":
				role, _ := message["role"].(string)
				if role == "" {
					role = "user"
				}
				content, err := responsesContentToChatContent(message["content"])
				if err != nil {
					return nil, err
				}
				messages = append(messages, map[string]any{"role": role, "content": content})
			case "function_call":
				toolCall, err := responsesFunctionCallToChatToolCall(message, 0)
				if err != nil {
					return nil, err
				}
				messages = append(messages, map[string]any{
					"role":       "assistant",
					"content":    "",
					"tool_calls": []map[string]any{toolCall},
				})
			case "function_call_output":
				callID, _ := message["call_id"].(string)
				if callID == "" {
					return nil, errors.New("Responses function_call_output 缺少 call_id，无法转换")
				}
				content, err := functionOutputToString(message["output"])
				if err != nil {
					return nil, err
				}
				messages = append(messages, map[string]any{
					"role":         "tool",
					"tool_call_id": callID,
					"content":      content,
				})
			default:
				return nil, fmt.Errorf("Responses input item 类型 %q 无法安全转换", itemType)
			}
		}
	default:
		return nil, errors.New("Responses input 只支持字符串或 message/item 数组")
	}
	out["messages"] = messages
	return out, nil
}

func convertChatToResponses(input map[string]any) (map[string]any, error) {
	allowed := map[string]bool{
		"model": true, "messages": true, "stream": true, "stream_options": true,
		"temperature": true, "top_p": true, "max_completion_tokens": true, "max_tokens": true,
		"stop": true, "user": true, "metadata": true, "presence_penalty": true, "frequency_penalty": true,
		"tools": true, "tool_choice": true, "parallel_tool_calls": true,
		"reasoning_effort": true, "reasoning": true,
	}
	for key := range input {
		if !allowed[key] {
			return nil, fmt.Errorf("当前上游不支持 Chat Completions 字段 %q，且无法安全转换", key)
		}
	}
	out := map[string]any{}
	for _, key := range []string{"model", "stream", "temperature", "top_p", "stop", "user", "metadata", "presence_penalty", "frequency_penalty"} {
		if value, ok := input[key]; ok {
			out[key] = value
		}
	}
	if value, ok := input["max_completion_tokens"]; ok {
		out["max_output_tokens"] = value
	} else if value, ok := input["max_tokens"]; ok {
		out["max_output_tokens"] = value
	}
	if tools, ok := input["tools"]; ok {
		converted, err := chatToolsToResponses(tools)
		if err != nil {
			return nil, err
		}
		out["tools"] = converted
	}
	if choice, ok := input["tool_choice"]; ok {
		converted, err := chatToolChoiceToResponses(choice)
		if err != nil {
			return nil, err
		}
		out["tool_choice"] = converted
	}
	if value, ok := input["parallel_tool_calls"]; ok {
		out["parallel_tool_calls"] = value
	}
	if value, ok := input["reasoning"]; ok {
		out["reasoning"] = value
	} else if value, ok := input["reasoning_effort"]; ok {
		out["reasoning"] = map[string]any{"effort": value}
	}
	if value, ok := input["stream_options"]; ok {
		if err := validateChatStreamOptions(value); err != nil {
			return nil, err
		}
	}
	rawMessages, ok := input["messages"]
	if !ok {
		return nil, errors.New("Chat Completions 请求缺少 messages，无法转换")
	}
	messageItems, ok := rawMessages.([]any)
	if !ok {
		return nil, errors.New("Chat Completions messages 只支持简单 message 数组")
	}
	instructions := []string{}
	messages := []map[string]any{}
	for _, item := range messageItems {
		message, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("Chat Completions messages 只支持简单 message 数组")
		}
		for key := range message {
			if key != "role" && key != "content" && key != "tool_calls" && key != "tool_call_id" && key != "name" &&
				key != "reasoning_content" && key != "reasoning" && key != "refusal" && key != "annotations" {
				return nil, fmt.Errorf("当前上游不支持 Chat Completions message 字段 %q，且无法安全转换", key)
			}
		}
		role, _ := message["role"].(string)
		if role == "" {
			role = "user"
		}
		switch role {
		case "system", "developer":
			content, err := chatContentToInstruction(message["content"])
			if err != nil {
				return nil, err
			}
			if content != "" {
				instructions = append(instructions, content)
			}
		case "user", "assistant":
			if contentValue, ok := message["content"]; ok && contentValue != nil {
				content, err := chatContentToResponsesContent(contentValue, role)
				if err != nil {
					return nil, err
				}
				if !emptyContent(content) {
					messages = append(messages, map[string]any{"role": role, "content": content})
				}
			} else if refusal, ok := message["refusal"].(string); ok && refusal != "" {
				messages = append(messages, map[string]any{"role": role, "content": refusal})
			}
			if role == "assistant" {
				toolCalls, err := chatMessageToolCallsToResponses(message["tool_calls"])
				if err != nil {
					return nil, err
				}
				messages = append(messages, toolCalls...)
			}
		case "tool":
			callID, _ := message["tool_call_id"].(string)
			if callID == "" {
				return nil, errors.New("Chat Completions tool message 缺少 tool_call_id，无法转换")
			}
			output, err := functionOutputToString(message["content"])
			if err != nil {
				return nil, err
			}
			messages = append(messages, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			})
		default:
			return nil, fmt.Errorf("Chat Completions role %q 无法安全转换", role)
		}
	}
	if len(instructions) > 0 {
		out["instructions"] = strings.Join(instructions, "\n")
	}
	out["input"] = messages
	return out, nil
}

func validateChatStreamOptions(value any) error {
	options, ok := value.(map[string]any)
	if !ok {
		return errors.New("Chat Completions stream_options 只支持对象，无法转换")
	}
	for key, raw := range options {
		if key != "include_usage" {
			return fmt.Errorf("Chat Completions stream_options.%s 无法安全转换", key)
		}
		if _, ok := raw.(bool); !ok {
			return errors.New("Chat Completions stream_options.include_usage 只支持布尔值")
		}
	}
	return nil
}

func chatStreamIncludeUsage(payload map[string]any) bool {
	options, _ := payload["stream_options"].(map[string]any)
	includeUsage, _ := options["include_usage"].(bool)
	return includeUsage
}

func reasoningEffort(value any) (any, bool) {
	switch reasoning := value.(type) {
	case map[string]any:
		effort, ok := reasoning["effort"]
		return effort, ok
	default:
		return nil, false
	}
}

func responsesToolsToChat(value any) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("Responses tools 只支持数组，无法转换")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		tool, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("Responses tools 只支持对象数组，无法转换")
		}
		toolType, _ := tool["type"].(string)
		if toolType == "" {
			toolType = "function"
		}
		if toolType != "function" {
			return nil, fmt.Errorf("Responses tool 类型 %q 无法安全转换到 Chat Completions", toolType)
		}
		if fn, ok := tool["function"].(map[string]any); ok {
			out = append(out, map[string]any{"type": "function", "function": cloneMap(fn)})
			continue
		}
		name, _ := tool["name"].(string)
		if name == "" {
			return nil, errors.New("Responses function tool 缺少 name，无法转换")
		}
		fn := map[string]any{"name": name}
		for _, key := range []string{"description", "parameters", "strict"} {
			if value, ok := tool[key]; ok {
				fn[key] = value
			}
		}
		out = append(out, map[string]any{"type": "function", "function": fn})
	}
	return out, nil
}

func chatToolsToResponses(value any) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("Chat Completions tools 只支持数组，无法转换")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		tool, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("Chat Completions tools 只支持对象数组，无法转换")
		}
		toolType, _ := tool["type"].(string)
		if toolType == "" {
			toolType = "function"
		}
		if toolType != "function" {
			return nil, fmt.Errorf("Chat Completions tool 类型 %q 无法安全转换到 Responses", toolType)
		}
		fn, _ := tool["function"].(map[string]any)
		if fn == nil {
			fn = tool
		}
		name, _ := fn["name"].(string)
		if name == "" {
			return nil, errors.New("Chat Completions function tool 缺少 function.name，无法转换")
		}
		converted := map[string]any{"type": "function", "name": name}
		for _, key := range []string{"description", "parameters", "strict"} {
			if value, ok := fn[key]; ok {
				converted[key] = value
			}
		}
		out = append(out, converted)
	}
	return out, nil
}

func responsesToolChoiceToChat(value any) (any, error) {
	switch choice := value.(type) {
	case string:
		return choice, nil
	case map[string]any:
		choiceType, _ := choice["type"].(string)
		if choiceType != "function" {
			return nil, fmt.Errorf("Responses tool_choice 类型 %q 无法安全转换到 Chat Completions", choiceType)
		}
		if fn, ok := choice["function"].(map[string]any); ok {
			return map[string]any{"type": "function", "function": cloneMap(fn)}, nil
		}
		name, _ := choice["name"].(string)
		if name == "" {
			return nil, errors.New("Responses function tool_choice 缺少 name，无法转换")
		}
		return map[string]any{"type": "function", "function": map[string]any{"name": name}}, nil
	default:
		return nil, errors.New("Responses tool_choice 格式无法转换")
	}
}

func chatToolChoiceToResponses(value any) (any, error) {
	switch choice := value.(type) {
	case string:
		return choice, nil
	case map[string]any:
		choiceType, _ := choice["type"].(string)
		if choiceType != "function" {
			return nil, fmt.Errorf("Chat Completions tool_choice 类型 %q 无法安全转换到 Responses", choiceType)
		}
		if name, ok := choice["name"].(string); ok && name != "" {
			return map[string]any{"type": "function", "name": name}, nil
		}
		fn, _ := choice["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if name == "" {
			return nil, errors.New("Chat Completions function tool_choice 缺少 function.name，无法转换")
		}
		return map[string]any{"type": "function", "name": name}, nil
	default:
		return nil, errors.New("Chat Completions tool_choice 格式无法转换")
	}
}

func responsesContentToChatContent(value any) (any, error) {
	switch content := value.(type) {
	case nil:
		return "", nil
	case string:
		return content, nil
	case []any:
		parts := []map[string]any{}
		texts := []string{}
		allText := true
		for _, item := range content {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("Responses content 只支持文本或图片对象")
			}
			part, text, isText, err := responsesContentPartToChat(obj)
			if err != nil {
				return nil, err
			}
			if isText {
				texts = append(texts, text)
			} else {
				allText = false
			}
			parts = append(parts, part)
		}
		if allText {
			return strings.Join(texts, "\n"), nil
		}
		return parts, nil
	default:
		return nil, errors.New("Responses content 只支持文本或图片")
	}
}

func responsesContentPartToChat(obj map[string]any) (map[string]any, string, bool, error) {
	partType, _ := obj["type"].(string)
	switch partType {
	case "", "text", "input_text", "output_text":
		text := firstString(obj, "text", "input_text", "output_text")
		return map[string]any{"type": "text", "text": text}, text, true, nil
	case "input_image":
		url := firstString(obj, "image_url", "url")
		if url == "" {
			return nil, "", false, errors.New("Responses input_image 缺少 image_url，无法转换到 Chat Completions")
		}
		image := map[string]any{"url": url}
		if detail, ok := obj["detail"]; ok {
			image["detail"] = detail
		}
		return map[string]any{"type": "image_url", "image_url": image}, "", false, nil
	case "refusal":
		text := firstString(obj, "refusal", "text")
		return map[string]any{"type": "text", "text": text}, text, true, nil
	default:
		return nil, "", false, fmt.Errorf("Responses content 类型 %q 无法安全转换", partType)
	}
}

func chatContentToResponsesContent(value any, role string) (any, error) {
	switch content := value.(type) {
	case nil:
		return "", nil
	case string:
		return content, nil
	case []any:
		parts := []map[string]any{}
		for _, item := range content {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("Chat Completions content 只支持文本或图片对象")
			}
			part, err := chatContentPartToResponses(obj, role)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
		return parts, nil
	default:
		return nil, errors.New("Chat Completions content 只支持文本或图片")
	}
}

func chatContentPartToResponses(obj map[string]any, role string) (map[string]any, error) {
	partType, _ := obj["type"].(string)
	switch partType {
	case "text":
		return map[string]any{"type": responsesTextPartType(role), "text": firstString(obj, "text")}, nil
	case "image_url":
		if role != "user" {
			return nil, errors.New("只有 user 消息里的图片能转换到 Responses")
		}
		image, ok := obj["image_url"].(map[string]any)
		if !ok {
			if url, ok := obj["image_url"].(string); ok && url != "" {
				return map[string]any{"type": "input_image", "image_url": url}, nil
			}
			return nil, errors.New("Chat Completions image_url 格式无法转换")
		}
		url, _ := image["url"].(string)
		if url == "" {
			return nil, errors.New("Chat Completions image_url 缺少 url，无法转换")
		}
		out := map[string]any{"type": "input_image", "image_url": url}
		if detail, ok := image["detail"]; ok {
			out["detail"] = detail
		}
		return out, nil
	case "input_text", "output_text":
		return map[string]any{"type": partType, "text": firstString(obj, "text", "input_text", "output_text")}, nil
	case "input_image":
		if role != "user" {
			return nil, errors.New("只有 user 消息里的图片能转换到 Responses")
		}
		out := cloneMap(obj)
		if out["type"] == nil {
			out["type"] = "input_image"
		}
		return out, nil
	default:
		return nil, fmt.Errorf("Chat Completions content 类型 %q 无法安全转换", partType)
	}
}

func responsesTextPartType(role string) string {
	if role == "assistant" {
		return "output_text"
	}
	return "input_text"
}

func chatContentToInstruction(value any) (string, error) {
	switch content := value.(type) {
	case nil:
		return "", nil
	case string:
		return content, nil
	case []any:
		parts := []string{}
		for _, item := range content {
			obj, ok := item.(map[string]any)
			if !ok {
				return "", errors.New("system/developer content 只支持文本")
			}
			partType, _ := obj["type"].(string)
			if partType != "text" && partType != "input_text" && partType != "" {
				return "", errors.New("system/developer content 只支持文本")
			}
			parts = append(parts, firstString(obj, "text", "input_text"))
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", errors.New("system/developer content 只支持文本")
	}
}

func chatMessageToolCallsToResponses(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("Chat Completions tool_calls 只支持数组，无法转换")
	}
	out := make([]map[string]any, 0, len(items))
	for i, item := range items {
		toolCall, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("Chat Completions tool_calls 只支持对象数组，无法转换")
		}
		callType, _ := toolCall["type"].(string)
		if callType == "" {
			callType = "function"
		}
		if callType != "function" {
			return nil, fmt.Errorf("Chat Completions tool_call 类型 %q 无法安全转换", callType)
		}
		fn, _ := toolCall["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if name == "" {
			return nil, errors.New("Chat Completions tool_call 缺少 function.name，无法转换")
		}
		callID, _ := toolCall["id"].(string)
		if callID == "" {
			callID = "call_" + strconv.Itoa(i)
		}
		arguments, err := functionOutputToString(fn["arguments"])
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"type":      "function_call",
			"call_id":   callID,
			"name":      name,
			"arguments": arguments,
		})
	}
	return out, nil
}

func responsesFunctionCallToChatToolCall(item map[string]any, index int) (map[string]any, error) {
	callID := firstString(item, "call_id", "id")
	if callID == "" {
		callID = "call_" + strconv.Itoa(index)
	}
	name, _ := item["name"].(string)
	if name == "" {
		return nil, errors.New("Responses function_call 缺少 name，无法转换")
	}
	arguments, err := functionOutputToString(item["arguments"])
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":   callID,
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}, nil
}

func functionOutputToString(value any) (string, error) {
	switch output := value.(type) {
	case nil:
		return "", nil
	case string:
		return output, nil
	default:
		payload, err := json.Marshal(output)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	}
}

func emptyContent(value any) bool {
	switch content := value.(type) {
	case nil:
		return true
	case string:
		return content == ""
	case []any:
		return len(content) == 0
	case []map[string]any:
		return len(content) == 0
	default:
		return false
	}
}

func firstString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok {
			return value
		}
	}
	return ""
}

func convertResponsePayload(payload []byte, conversion conversionMode) ([]byte, error) {
	switch conversion {
	case conversionResponsesToChat:
		return convertChatResponseToResponses(payload)
	case conversionChatToResponses:
		return convertResponsesResponseToChat(payload)
	default:
		return payload, nil
	}
}

func convertChatResponseToResponses(payload []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("上游 Chat Completions 返回不是有效 JSON：%w", err)
	}
	choice, err := firstChatChoice(root)
	if err != nil {
		return nil, err
	}
	finishReason, _ := choice["finish_reason"].(string)
	createdAt := numberToInt64(root["created"])
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	status := "completed"
	if finishReason == "length" {
		status = "incomplete"
	}
	output, outputText, err := chatChoiceToResponsesOutput(choice, status)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"id":          responseID(root["id"]),
		"object":      "response",
		"created_at":  createdAt,
		"status":      status,
		"model":       root["model"],
		"output":      output,
		"output_text": outputText,
	}
	if usage := chatUsageToResponses(root["usage"]); len(usage) > 0 {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

func convertResponsesResponseToChat(payload []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("上游 Responses 返回不是有效 JSON：%w", err)
	}
	created := numberToInt64(root["created"])
	if created == 0 {
		created = numberToInt64(root["created_at"])
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	message, finishReason, err := responsesOutputToChatMessage(root)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"id":      chatCompletionID(root["id"]),
		"object":  "chat.completion",
		"created": created,
		"model":   root["model"],
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
	}
	if usage := responsesUsageToChat(root["usage"]); len(usage) > 0 {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

func firstChatChoice(root map[string]any) (map[string]any, error) {
	choices, ok := root["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, errors.New("上游 Chat Completions 返回缺少 choices，无法转换")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, errors.New("上游 Chat Completions choices 格式无法转换")
	}
	return choice, nil
}

func chatChoiceToResponsesOutput(choice map[string]any, status string) ([]any, string, error) {
	message, _ := choice["message"].(map[string]any)
	if message == nil {
		return nil, "", errors.New("上游 Chat Completions 返回缺少 message，无法转换")
	}
	role, _ := message["role"].(string)
	if role == "" {
		role = "assistant"
	}
	output := []any{}
	outputText, err := chatMessageContent(message)
	if err != nil {
		return nil, "", err
	}
	if outputText != "" {
		output = append(output, map[string]any{
			"type":   "message",
			"id":     "msg_0",
			"status": status,
			"role":   role,
			"content": []map[string]any{{
				"type":        "output_text",
				"text":        outputText,
				"annotations": []any{},
			}},
		})
	}
	if reasoningText := chatReasoningContent(message); reasoningText != "" {
		output = append([]any{map[string]any{
			"type":   "reasoning",
			"id":     "rs_0",
			"status": status,
			"summary": []map[string]any{{
				"type": "summary_text",
				"text": reasoningText,
			}},
		}}, output...)
	}
	toolCalls, err := chatMessageToolCallsToResponses(message["tool_calls"])
	if err != nil {
		return nil, "", err
	}
	for i, toolCall := range toolCalls {
		item := cloneMap(toolCall)
		if _, ok := item["id"]; !ok {
			item["id"] = "fc_" + strconv.Itoa(i)
		}
		item["status"] = status
		output = append(output, item)
	}
	if len(output) == 0 {
		output = append(output, map[string]any{
			"type":   "message",
			"id":     "msg_0",
			"status": status,
			"role":   role,
			"content": []map[string]any{{
				"type":        "output_text",
				"text":        "",
				"annotations": []any{},
			}},
		})
	}
	return output, outputText, nil
}

func chatMessageContent(message map[string]any) (string, error) {
	if content, ok := message["content"]; ok && content != nil {
		return simpleContent(content)
	}
	if refusal, ok := message["refusal"].(string); ok {
		return refusal, nil
	}
	return "", nil
}

func chatReasoningContent(message map[string]any) string {
	if text, ok := message["reasoning_content"].(string); ok {
		return text
	}
	switch reasoning := message["reasoning"].(type) {
	case string:
		return reasoning
	case map[string]any:
		return firstString(reasoning, "content", "text", "summary")
	default:
		return ""
	}
}

func responsesOutputToChatMessage(root map[string]any) (map[string]any, string, error) {
	content := responsesOutputText(root)
	reasoningContent := responsesReasoningContent(root)
	toolCalls := []map[string]any{}
	output, _ := root["output"].([]any)
	for i, item := range output {
		obj, _ := item.(map[string]any)
		itemType, _ := obj["type"].(string)
		if itemType != "function_call" {
			continue
		}
		toolCall, err := responsesFunctionCallToChatToolCall(obj, i)
		if err != nil {
			return nil, "", err
		}
		toolCalls = append(toolCalls, toolCall)
	}
	message := map[string]any{
		"role":    "assistant",
		"content": content,
	}
	if reasoningContent != "" {
		message["reasoning_content"] = reasoningContent
	}
	finishReason := finishReasonFromResponseStatus(root["status"])
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		finishReason = "tool_calls"
		if content == "" {
			message["content"] = nil
		}
	}
	return message, finishReason, nil
}

func responsesReasoningContent(root map[string]any) string {
	parts := []string{}
	output, _ := root["output"].([]any)
	for _, item := range output {
		obj, _ := item.(map[string]any)
		itemType, _ := obj["type"].(string)
		if itemType != "reasoning" {
			continue
		}
		if text := firstString(obj, "text", "content"); text != "" {
			parts = append(parts, text)
		}
		summaryItems, _ := obj["summary"].([]any)
		for _, rawSummary := range summaryItems {
			summary, _ := rawSummary.(map[string]any)
			if text := firstString(summary, "text", "summary_text"); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func responsesOutputText(root map[string]any) string {
	parts := []string{}
	if text, ok := root["output_text"].(string); ok && text != "" {
		parts = append(parts, text)
	}
	output, _ := root["output"].([]any)
	for _, item := range output {
		message, _ := item.(map[string]any)
		itemType, _ := message["type"].(string)
		if itemType != "" && itemType != "message" {
			continue
		}
		contentItems, _ := message["content"].([]any)
		for _, contentItem := range contentItems {
			content, _ := contentItem.(map[string]any)
			if text, ok := content["text"].(string); ok {
				parts = append(parts, text)
				continue
			}
			if text, ok := content["output_text"].(string); ok {
				parts = append(parts, text)
				continue
			}
			if refusal, ok := content["refusal"].(string); ok {
				parts = append(parts, refusal)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func chatUsageToResponses(value any) map[string]any {
	usage, _ := value.(map[string]any)
	if usage == nil {
		return nil
	}
	inputTokens := numberToInt64(usage["prompt_tokens"])
	outputTokens := numberToInt64(usage["completion_tokens"])
	totalTokens := numberToInt64(usage["total_tokens"])
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	if inputTokens == 0 && outputTokens == 0 && totalTokens == 0 {
		return nil
	}
	return map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"total_tokens":  totalTokens,
	}
}

func responsesUsageToChat(value any) map[string]any {
	usage, _ := value.(map[string]any)
	if usage == nil {
		return nil
	}
	promptTokens := numberToInt64(usage["input_tokens"])
	completionTokens := numberToInt64(usage["output_tokens"])
	totalTokens := numberToInt64(usage["total_tokens"])
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	if promptTokens == 0 && completionTokens == 0 && totalTokens == 0 {
		return nil
	}
	return map[string]any{
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
	}
}

func responseID(value any) string {
	id, _ := value.(string)
	if id == "" {
		return "resp_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if strings.HasPrefix(id, "resp_") {
		return id
	}
	return "resp_" + id
}

func chatCompletionID(value any) string {
	id, _ := value.(string)
	if id == "" {
		return "chatcmpl_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if strings.HasPrefix(id, "chatcmpl_") {
		return id
	}
	return "chatcmpl_" + id
}

func finishReasonFromResponseStatus(value any) string {
	status, _ := value.(string)
	if status == "incomplete" {
		return "length"
	}
	return "stop"
}

func simpleContent(value any) (string, error) {
	switch content := value.(type) {
	case string:
		return content, nil
	case []any:
		parts := []string{}
		for _, item := range content {
			obj, ok := item.(map[string]any)
			if !ok {
				return "", errors.New("Responses content 只支持文本")
			}
			if text, ok := obj["text"].(string); ok {
				parts = append(parts, text)
				continue
			}
			if text, ok := obj["input_text"].(string); ok {
				parts = append(parts, text)
				continue
			}
			return "", errors.New("Responses content 只支持文本")
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", errors.New("Responses content 只支持文本")
	}
}

func convertStream(w http.ResponseWriter, body io.Reader, conversion conversionMode, model string, includeUsage bool) error {
	switch conversion {
	case conversionResponsesToChat:
		return streamChatToResponses(w, body, model)
	case conversionChatToResponses:
		return streamResponsesToChat(w, body, model, includeUsage)
	default:
		_, err := io.Copy(w, body)
		return err
	}
}

func streamChatToResponses(w http.ResponseWriter, body io.Reader, model string) error {
	responseID := "resp_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	createdAt := time.Now().Unix()
	createdSent := false
	completedSent := false
	toolCalls := map[int]*streamToolCallState{}
	ensureCreated := func() error {
		if createdSent {
			return nil
		}
		createdSent = true
		return writeSSE(w, "response.created", map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":         responseID,
				"object":     "response",
				"created_at": createdAt,
				"status":     "in_progress",
				"model":      model,
				"output":     []any{},
			},
		})
	}
	complete := func() error {
		if completedSent {
			return nil
		}
		for _, state := range toolCalls {
			if state.Added && !state.Done {
				if err := writeSSE(w, "response.function_call_arguments.done", map[string]any{
					"type":         "response.function_call_arguments.done",
					"response_id":  responseID,
					"item_id":      state.ItemID,
					"output_index": state.Index,
					"arguments":    state.Arguments,
				}); err != nil {
					return err
				}
				if err := writeSSE(w, "response.output_item.done", map[string]any{
					"type":         "response.output_item.done",
					"response_id":  responseID,
					"output_index": state.Index,
					"item": map[string]any{
						"type":      "function_call",
						"id":        state.ItemID,
						"call_id":   state.CallID,
						"name":      state.Name,
						"arguments": state.Arguments,
						"status":    "completed",
					},
				}); err != nil {
					return err
				}
				state.Done = true
			}
		}
		completedSent = true
		return writeSSE(w, "response.completed", map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":         responseID,
				"object":     "response",
				"created_at": createdAt,
				"status":     "completed",
				"model":      model,
				"output":     []any{},
			},
		})
	}
	return readSSE(body, func(_ string, data string) error {
		if data == "[DONE]" {
			return complete()
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		if id, ok := chunk["id"].(string); ok && id != "" && strings.HasPrefix(id, "resp_") {
			responseID = id
		}
		if value, ok := chunk["created"].(float64); ok && value > 0 {
			createdAt = int64(value)
		}
		if value, ok := chunk["model"].(string); ok && value != "" {
			model = value
		}
		if err := ensureCreated(); err != nil {
			return err
		}
		choices, _ := chunk["choices"].([]any)
		for _, rawChoice := range choices {
			choice, _ := rawChoice.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			if text := chatReasoningDelta(delta); text != "" {
				if err := writeSSE(w, "response.reasoning_summary_text.delta", map[string]any{
					"type":          "response.reasoning_summary_text.delta",
					"response_id":   responseID,
					"item_id":       "rs_0",
					"output_index":  0,
					"summary_index": 0,
					"delta":         text,
				}); err != nil {
					return err
				}
			}
			if text, ok := delta["content"].(string); ok && text != "" {
				if err := writeSSE(w, "response.output_text.delta", map[string]any{
					"type":          "response.output_text.delta",
					"response_id":   responseID,
					"item_id":       "msg_0",
					"output_index":  0,
					"content_index": 0,
					"delta":         text,
				}); err != nil {
					return err
				}
			}
			if rawToolCalls, ok := delta["tool_calls"].([]any); ok {
				for _, rawToolCall := range rawToolCalls {
					toolCall, _ := rawToolCall.(map[string]any)
					index := int(numberToInt64(toolCall["index"]))
					state := toolCalls[index]
					if state == nil {
						state = &streamToolCallState{
							Index:  index,
							ItemID: "fc_" + strconv.Itoa(index),
							CallID: "call_" + strconv.Itoa(index),
						}
						toolCalls[index] = state
					}
					if id, ok := toolCall["id"].(string); ok && id != "" {
						state.CallID = id
					}
					fn, _ := toolCall["function"].(map[string]any)
					if name, ok := fn["name"].(string); ok && name != "" {
						state.Name = name
					}
					if !state.Added && state.Name != "" {
						state.Added = true
						if err := writeSSE(w, "response.output_item.added", map[string]any{
							"type":         "response.output_item.added",
							"response_id":  responseID,
							"output_index": state.Index,
							"item": map[string]any{
								"type":      "function_call",
								"id":        state.ItemID,
								"call_id":   state.CallID,
								"name":      state.Name,
								"arguments": "",
								"status":    "in_progress",
							},
						}); err != nil {
							return err
						}
					}
					if arguments, ok := fn["arguments"].(string); ok && arguments != "" {
						state.Arguments += arguments
						if !state.Added {
							state.Added = true
							if err := writeSSE(w, "response.output_item.added", map[string]any{
								"type":         "response.output_item.added",
								"response_id":  responseID,
								"output_index": state.Index,
								"item": map[string]any{
									"type":      "function_call",
									"id":        state.ItemID,
									"call_id":   state.CallID,
									"name":      state.Name,
									"arguments": "",
									"status":    "in_progress",
								},
							}); err != nil {
								return err
							}
						}
						if err := writeSSE(w, "response.function_call_arguments.delta", map[string]any{
							"type":         "response.function_call_arguments.delta",
							"response_id":  responseID,
							"item_id":      state.ItemID,
							"output_index": state.Index,
							"delta":        arguments,
						}); err != nil {
							return err
						}
					}
				}
			}
			if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" && !completedSent {
				return complete()
			}
		}
		return nil
	})
}

func chatReasoningDelta(delta map[string]any) string {
	if text, ok := delta["reasoning_content"].(string); ok {
		return text
	}
	if text, ok := delta["reasoning"].(string); ok {
		return text
	}
	return ""
}

type streamToolCallState struct {
	Index     int
	ItemID    string
	CallID    string
	Name      string
	Arguments string
	Added     bool
	Done      bool
}

func streamResponsesToChat(w http.ResponseWriter, body io.Reader, model string, includeUsage bool) error {
	chatID := "chatcmpl_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	created := time.Now().Unix()
	roleSent := false
	doneSent := false
	toolCallSeen := false
	nextToolIndex := 0
	toolIndexes := map[string]int{}
	toolIndex := func(itemID string, outputIndex int64) int {
		if itemID != "" {
			if index, ok := toolIndexes[itemID]; ok {
				return index
			}
			index := nextToolIndex
			nextToolIndex++
			toolIndexes[itemID] = index
			return index
		}
		if outputIndex >= 0 {
			return int(outputIndex)
		}
		index := nextToolIndex
		nextToolIndex++
		return index
	}
	return readSSE(body, func(event string, data string) error {
		if data == "[DONE]" {
			if !doneSent {
				doneSent = true
				_, err := io.WriteString(w, "data: [DONE]\n\n")
				flush(w)
				return err
			}
			return nil
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		if response, ok := chunk["response"].(map[string]any); ok {
			if id, ok := response["id"].(string); ok && id != "" {
				chatID = chatCompletionID(id)
			}
			if value := numberToInt64(response["created_at"]); value > 0 {
				created = value
			}
			if value, ok := response["model"].(string); ok && value != "" {
				model = value
			}
		}
		if id, ok := chunk["response_id"].(string); ok && id != "" {
			chatID = chatCompletionID(id)
		}
		eventType, _ := chunk["type"].(string)
		if eventType == "" {
			eventType = event
		}
		if eventType == "response.output_text.delta" {
			text, _ := chunk["delta"].(string)
			delta := map[string]any{}
			if !roleSent {
				roleSent = true
				delta["role"] = "assistant"
			}
			if text != "" {
				delta["content"] = text
			}
			if len(delta) > 0 {
				return writeChatChunk(w, chatID, created, model, delta, nil)
			}
		}
		if eventType == "response.reasoning_summary_text.delta" || eventType == "response.reasoning_text.delta" {
			text, _ := chunk["delta"].(string)
			delta := map[string]any{}
			if !roleSent {
				roleSent = true
				delta["role"] = "assistant"
			}
			if text != "" {
				delta["reasoning_content"] = text
			}
			if len(delta) > 0 {
				return writeChatChunk(w, chatID, created, model, delta, nil)
			}
		}
		if eventType == "response.output_item.added" {
			item, _ := chunk["item"].(map[string]any)
			if itemType, _ := item["type"].(string); itemType == "function_call" {
				toolCallSeen = true
				itemID, _ := item["id"].(string)
				outputIndex := numberToInt64(chunk["output_index"])
				index := toolIndex(itemID, outputIndex)
				callID := firstString(item, "call_id", "id")
				name, _ := item["name"].(string)
				delta := map[string]any{
					"tool_calls": []map[string]any{{
						"index": index,
						"id":    callID,
						"type":  "function",
						"function": map[string]any{
							"name":      name,
							"arguments": "",
						},
					}},
				}
				if !roleSent {
					roleSent = true
					delta["role"] = "assistant"
				}
				return writeChatChunk(w, chatID, created, model, delta, nil)
			}
		}
		if eventType == "response.function_call_arguments.delta" {
			toolCallSeen = true
			itemID, _ := chunk["item_id"].(string)
			outputIndex := numberToInt64(chunk["output_index"])
			index := toolIndex(itemID, outputIndex)
			arguments, _ := chunk["delta"].(string)
			delta := map[string]any{
				"tool_calls": []map[string]any{{
					"index": index,
					"function": map[string]any{
						"arguments": arguments,
					},
				}},
			}
			if !roleSent {
				roleSent = true
				delta["role"] = "assistant"
			}
			return writeChatChunk(w, chatID, created, model, delta, nil)
		}
		if eventType == "response.completed" && !doneSent {
			doneSent = true
			if !roleSent {
				roleSent = true
				if err := writeChatChunk(w, chatID, created, model, map[string]any{"role": "assistant"}, nil); err != nil {
					return err
				}
			}
			finishReason := "stop"
			if toolCallSeen {
				finishReason = "tool_calls"
			}
			if err := writeChatChunk(w, chatID, created, model, map[string]any{}, finishReason); err != nil {
				return err
			}
			if includeUsage {
				if usage := chatUsageFromResponseCompleted(chunk); len(usage) > 0 {
					if err := writeChatUsageChunk(w, chatID, created, model, usage); err != nil {
						return err
					}
				}
			}
			_, err := io.WriteString(w, "data: [DONE]\n\n")
			flush(w)
			return err
		}
		return nil
	})
}

func readSSE(body io.Reader, handle func(event string, data string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	event := ""
	dataLines := []string{}
	flushEvent := func() error {
		if len(dataLines) == 0 {
			event = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		currentEvent := event
		event = ""
		return handle(currentEvent, data)
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flushEvent(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimPrefix(data, " ")
			dataLines = append(dataLines, data)
		}
	}
	if err := flushEvent(); err != nil {
		return err
	}
	return scanner.Err()
}

func writeSSE(w io.Writer, event string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	flush(w)
	return nil
}

func writeChatChunk(w io.Writer, id string, created int64, model string, delta map[string]any, finishReason any) error {
	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": finishReason,
		}},
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	flush(w)
	return nil
}

func writeChatUsageChunk(w io.Writer, id string, created int64, model string, usage map[string]any) error {
	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []any{},
		"usage":   usage,
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	flush(w)
	return nil
}

func chatUsageFromResponseCompleted(chunk map[string]any) map[string]any {
	if response, ok := chunk["response"].(map[string]any); ok {
		if usage := responsesUsageToChat(response["usage"]); len(usage) > 0 {
			return usage
		}
	}
	return responsesUsageToChat(chunk["usage"])
}

func flush(w io.Writer) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func joinEndpoint(baseURL, endpoint string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return baseURL + endpoint
	}
	return baseURL + endpoint
}

func copyRequestHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopHeader(key) || strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Host") || strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func isFallbackable(status int, endpoint string) bool {
	if status == http.StatusTooManyRequests || status >= 500 || status == http.StatusRequestTimeout || status == statusClientClosedRequest {
		return true
	}
	return endpoint == "/responses" && status == http.StatusNotFound
}

func requestCanceled(ctx context.Context) bool {
	return ctx.Err() != nil
}

func downstreamCancelMessage(err error) string {
	if err == nil {
		return "下游请求已取消（客户端或前置代理）"
	}
	return "下游请求已取消（客户端或前置代理）：" + err.Error()
}

func requestTimedOut(ctx, parent context.Context) bool {
	return parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

type usageInfo struct {
	prompt     int64
	completion int64
	total      int64
}

func parseUsage(payload []byte) usageInfo {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return usageInfo{}
	}
	usage, _ := root["usage"].(map[string]any)
	info := usageInfo{
		prompt:     numberToInt64(usage["prompt_tokens"]),
		completion: numberToInt64(usage["completion_tokens"]),
		total:      numberToInt64(usage["total_tokens"]),
	}
	if info.prompt == 0 {
		info.prompt = numberToInt64(usage["input_tokens"])
	}
	if info.completion == 0 {
		info.completion = numberToInt64(usage["output_tokens"])
	}
	if info.total == 0 {
		info.total = info.prompt + info.completion
	}
	return info
}

func numberToInt64(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case json.Number:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	default:
		return 0
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func newRequestID() string {
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

var (
	errRouteNotFound         = errors.New("没有找到可用的路由模型或原始模型")
	errNoCandidate           = errors.New("路由中没有可用模型")
	errConversionUnsupported = errors.New("协议转换失败")
)

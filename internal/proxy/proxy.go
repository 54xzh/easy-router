package proxy

import (
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
	Provider     store.Provider
	Model        store.Model
	Endpoint     string
	Converted    bool
	RequestBody  []byte
	AttemptLabel string
}

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
					return nil, err
				} else if ok {
					out = append(out, item)
				}
			}
		}
	}
	if len(out) == 0 {
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
	if api == "chat" && !model.SupportsChat {
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
	converted := false
	if api == "responses" {
		if model.SupportsResponses {
			endpoint = "/responses"
		} else {
			nextPayload, err = convertResponsesToChat(nextPayload)
			if err != nil {
				return candidate{}, false, err
			}
			converted = true
		}
	}
	body, err := json.Marshal(nextPayload)
	if err != nil {
		return candidate{}, false, err
	}
	return candidate{
		Provider:     provider,
		Model:        model,
		Endpoint:     endpoint,
		Converted:    converted,
		RequestBody:  body,
		AttemptLabel: model.InternalID,
	}, true, nil
}

func convertResponsesToChat(input map[string]any) (map[string]any, error) {
	allowed := map[string]bool{
		"model": true, "input": true, "instructions": true, "stream": true,
		"temperature": true, "top_p": true, "max_output_tokens": true, "max_tokens": true,
		"stop": true, "user": true, "metadata": true, "presence_penalty": true, "frequency_penalty": true,
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
				return nil, errors.New("Responses input 只支持字符串或简单 message 数组")
			}
			role, _ := message["role"].(string)
			if role == "" {
				role = "user"
			}
			content, err := simpleContent(message["content"])
			if err != nil {
				return nil, err
			}
			messages = append(messages, map[string]any{"role": role, "content": content})
		}
	default:
		return nil, errors.New("Responses input 只支持字符串或简单 message 数组")
	}
	out["messages"] = messages
	return out, nil
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
	if info.total == 0 {
		info.total = numberToInt64(usage["input_tokens"]) + numberToInt64(usage["output_tokens"])
	}
	return info
}

func numberToInt64(value any) int64 {
	switch v := value.(type) {
	case float64:
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
	errRouteNotFound = errors.New("没有找到可用的路由模型或原始模型")
	errNoCandidate   = errors.New("路由中没有可用模型")
)

package admin

import (
	"encoding/json"
	"fmt"
	"io"
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

func Register(mux *http.ServeMux, db *store.Store) {
	h := &Handler{
		store: db,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	mux.HandleFunc("/api/admin/login", h.login)
	mux.HandleFunc("/api/admin/logout", h.requireAuth(h.logout))
	mux.HandleFunc("/api/admin/me", h.requireAuth(h.me))
	mux.HandleFunc("/api/admin/change-password", h.requireAuth(h.changePassword))
	mux.HandleFunc("/api/admin/", h.requireAuth(h.dispatch))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	token, err := h.store.LoginAdmin(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	http.SetCookie(w, sessionCookie(token, time.Now().Add(7*24*time.Hour)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("easy_router_session"); err == nil {
		h.store.DeleteAdminSession(cookie.Value)
	}
	http.SetCookie(w, sessionCookie("", time.Unix(0, 0)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"username": "admin"})
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "新密码至少 8 位"})
		return
	}
	if err := h.store.ChangeAdminPassword(req.OldPassword, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("easy_router_session")
		if err != nil || !h.store.ValidateAdminSession(cookie.Value) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "请先登录"})
			return
		}
		next(w, r)
	}
}

func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/")
	switch {
	case path == "providers":
		h.providers(w, r)
	case strings.HasPrefix(path, "providers/"):
		h.providerDetail(w, r, strings.TrimPrefix(path, "providers/"))
	case path == "models":
		h.models(w, r)
	case strings.HasPrefix(path, "models/"):
		h.modelDetail(w, r, strings.TrimPrefix(path, "models/"))
	case path == "groups":
		h.groups(w, r)
	case strings.HasPrefix(path, "groups/"):
		h.groupDetail(w, r, strings.TrimPrefix(path, "groups/"))
	case path == "routes":
		h.routes(w, r)
	case strings.HasPrefix(path, "routes/"):
		h.routeDetail(w, r, strings.TrimPrefix(path, "routes/"))
	case path == "logs":
		h.logs(w, r)
	case path == "settings":
		h.settings(w, r)
	case path == "proxy-keys":
		h.proxyKeys(w, r)
	case strings.HasPrefix(path, "proxy-keys/"):
		h.proxyKeyDetail(w, r, strings.TrimPrefix(path, "proxy-keys/"))
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "接口不存在"})
	}
}

func (h *Handler) providers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.store.ListProviders()
		writeResult(w, items, err)
	case http.MethodPost:
		var p store.Provider
		if !decodeJSON(w, r, &p) {
			return
		}
		if !p.Enabled {
			p.Enabled = true
		}
		saved, err := h.store.UpsertProvider(p)
		writeResult(w, saved, err)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func (h *Handler) providerDetail(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	id, _ := url.PathUnescape(parts[0])
	if len(parts) == 2 && parts[1] == "sync" && r.Method == http.MethodPost {
		h.discoverProviderModels(w, id)
		return
	}
	if len(parts) == 3 && parts[1] == "models" && parts[2] == "import" && r.Method == http.MethodPost {
		h.importProviderModels(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "models" && r.Method == http.MethodPost {
		var m store.Model
		if !decodeJSON(w, r, &m) {
			return
		}
		m.ProviderID = id
		m.Enabled = true
		saved, err := h.store.UpsertModel(m)
		writeResult(w, saved, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		provider, err := h.store.GetProvider(id, false)
		writeResult(w, provider, err)
	case http.MethodPut:
		var p store.Provider
		if !decodeJSON(w, r, &p) {
			return
		}
		p.ID = id
		saved, err := h.store.UpsertProvider(p)
		writeResult(w, saved, err)
	case http.MethodDelete:
		writeResult(w, map[string]any{"ok": true}, h.store.DeleteProvider(id))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

type discoveredModel struct {
	OriginalID        string `json:"original_id"`
	DisplayName       string `json:"display_name"`
	InternalID        string `json:"internal_id"`
	AlreadyImported   bool   `json:"already_imported"`
	SupportsChat      bool   `json:"supports_chat"`
	SupportsResponses bool   `json:"supports_responses"`
	SupportsStream    bool   `json:"supports_stream"`
}

func (h *Handler) discoverProviderModels(w http.ResponseWriter, providerID string) {
	provider, err := h.store.GetProvider(providerID, true)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(provider.BaseURL, "/")+"/models", nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	for key, value := range provider.ExtraHeaders {
		req.Header.Set(key, value)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": fmt.Sprintf("上游返回 %d：%s", resp.StatusCode, strings.TrimSpace(string(body)))})
		return
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "模型列表响应格式不正确"})
		return
	}
	existing, err := h.store.ListModels()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	imported := map[string]bool{}
	for _, model := range existing {
		imported[model.InternalID] = true
	}
	models := make([]discoveredModel, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.ID == "" {
			continue
		}
		internalID := store.InternalModelID(provider.ID, item.ID)
		models = append(models, discoveredModel{
			OriginalID:        item.ID,
			DisplayName:       item.ID,
			InternalID:        internalID,
			AlreadyImported:   imported[internalID],
			SupportsChat:      true,
			SupportsResponses: false,
			SupportsStream:    true,
		})
	}
	writeJSON(w, http.StatusOK, models)
}

func (h *Handler) importProviderModels(w http.ResponseWriter, r *http.Request, providerID string) {
	var req struct {
		Models []store.Model `json:"models"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	models := make([]store.Model, 0, len(req.Models))
	for _, model := range req.Models {
		if model.OriginalID == "" {
			continue
		}
		model.ProviderID = providerID
		if model.DisplayName == "" {
			model.DisplayName = model.OriginalID
		}
		if !model.SupportsChat && !model.SupportsResponses {
			model.SupportsChat = true
		}
		model.Enabled = true
		models = append(models, model)
	}
	if len(models) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请选择至少一个模型"})
		return
	}
	saved, err := h.store.ImportProviderModels(providerID, models)
	writeResult(w, saved, err)
}

func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	models, err := h.store.ListModels()
	writeResult(w, models, err)
}

func (h *Handler) modelDetail(w http.ResponseWriter, r *http.Request, rest string) {
	restore := strings.HasSuffix(rest, "/restore")
	idPart := strings.TrimSuffix(rest, "/restore")
	id, _ := url.PathUnescape(idPart)
	if restore && r.Method == http.MethodPost {
		writeResult(w, map[string]any{"ok": true}, h.store.RestoreModel(id))
		return
	}
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	current, err := h.store.GetModel(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	var patch store.Model
	if !decodeJSON(w, r, &patch) {
		return
	}
	if patch.DisplayName != "" {
		current.DisplayName = patch.DisplayName
	}
	current.SupportsChat = patch.SupportsChat
	current.SupportsResponses = patch.SupportsResponses
	current.SupportsStream = patch.SupportsStream
	current.ContextLength = patch.ContextLength
	current.Enabled = patch.Enabled
	saved, err := h.store.UpsertModel(current)
	writeResult(w, saved, err)
}

func (h *Handler) groups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := h.store.ListGroups()
		writeResult(w, groups, err)
	case http.MethodPost:
		var g store.ModelGroup
		if !decodeJSON(w, r, &g) {
			return
		}
		if !g.Enabled {
			g.Enabled = true
		}
		saved, err := h.store.UpsertGroup(g)
		writeResult(w, saved, err)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func (h *Handler) groupDetail(w http.ResponseWriter, r *http.Request, rest string) {
	id, _ := url.PathUnescape(rest)
	switch r.Method {
	case http.MethodGet:
		group, err := h.store.GetGroup(id)
		writeResult(w, group, err)
	case http.MethodPut:
		var g store.ModelGroup
		if !decodeJSON(w, r, &g) {
			return
		}
		g.ID = id
		saved, err := h.store.UpsertGroup(g)
		writeResult(w, saved, err)
	case http.MethodDelete:
		writeResult(w, map[string]any{"ok": true}, h.store.DeleteGroup(id))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func (h *Handler) routes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		routes, err := h.store.ListRoutes()
		writeResult(w, routes, err)
	case http.MethodPost:
		var route store.Route
		if !decodeJSON(w, r, &route) {
			return
		}
		if !route.Enabled {
			route.Enabled = true
		}
		saved, err := h.store.UpsertRoute(route)
		writeResult(w, saved, err)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func (h *Handler) routeDetail(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	id, _ := url.PathUnescape(parts[0])
	if len(parts) == 2 && parts[1] == "enabled" && r.Method == http.MethodPatch {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		writeResult(w, map[string]any{"ok": true}, h.store.SetRouteEnabled(id, req.Enabled))
		return
	}
	if len(parts) == 2 && parts[1] == "override" && r.Method == http.MethodPost {
		var req store.Override
		if !decodeJSON(w, r, &req) {
			return
		}
		req.RouteID = id
		writeResult(w, map[string]any{"ok": true}, h.store.SetRouteOverride(req))
		return
	}
	switch r.Method {
	case http.MethodGet:
		route, err := h.store.GetRoute(id)
		writeResult(w, route, err)
	case http.MethodPut:
		var route store.Route
		if !decodeJSON(w, r, &route) {
			return
		}
		route.ID = id
		saved, err := h.store.UpsertRoute(route)
		writeResult(w, saved, err)
	case http.MethodDelete:
		writeResult(w, map[string]any{"ok": true}, h.store.DeleteRoute(id))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs, err := h.store.ListLogs(limit)
	writeResult(w, logs, err)
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.store.Settings()
		writeResult(w, settings, err)
	case http.MethodPut:
		var req map[string]string
		if !decodeJSON(w, r, &req) {
			return
		}
		for key, value := range req {
			if err := h.store.SetSetting(key, value); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
		}
		settings, err := h.store.Settings()
		writeResult(w, settings, err)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func (h *Handler) proxyKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keys, err := h.store.ListProxyKeys()
		writeResult(w, keys, err)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			req.Name = "默认密钥"
		}
		key, err := h.store.CreateProxyKey(req.Name)
		if err != nil {
			writeResult(w, store.ProxyKey{}, err)
			return
		}
		writeJSON(w, http.StatusOK, key.ProxyKey)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func (h *Handler) proxyKeyDetail(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	id, _ := url.PathUnescape(parts[0])
	if len(parts) == 2 && parts[1] == "token" && r.Method == http.MethodGet {
		token, err := h.store.GetProxyKeyToken(id)
		writeResult(w, map[string]string{"token": token}, err)
		return
	}
	if len(parts) != 1 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "接口不存在"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		writeResult(w, map[string]any{"ok": true}, h.store.SetProxyKeyEnabled(id, req.Enabled))
	case http.MethodDelete:
		writeResult(w, map[string]any{"ok": true}, h.store.DeleteProxyKey(id))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
	}
}

func sessionCookie(value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     "easy_router_session",
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求 JSON 格式不正确"})
		return false
	}
	return true
}

func writeResult[T any](w http.ResponseWriter, value T, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) ListProviders() ([]Provider, error) {
	rows, err := s.db.Query(`SELECT id, name, base_url, extra_headers_json, enabled, created_at, updated_at FROM providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := []Provider{}
	for rows.Next() {
		var p Provider
		var headers string
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &headers, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.ExtraHeaders = unmarshalHeaders(headers)
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (s *Store) GetProvider(id string, includeKey bool) (Provider, error) {
	var p Provider
	var headers, cipher string
	err := s.db.QueryRow(`SELECT id, name, base_url, api_key_cipher, extra_headers_json, enabled, created_at, updated_at FROM providers WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.BaseURL, &cipher, &headers, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Provider{}, err
	}
	p.ExtraHeaders = unmarshalHeaders(headers)
	if includeKey {
		key, err := decryptSecret(s.secretKey, cipher)
		if err != nil {
			return Provider{}, err
		}
		p.APIKey = key
	}
	return p, nil
}

func (s *Store) UpsertProvider(p Provider) (Provider, error) {
	if p.ID == "" {
		return Provider{}, errors.New("provider_id 不能为空")
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.BaseURL == "" {
		return Provider{}, errors.New("base_url 不能为空")
	}
	headers, err := marshalHeaders(p.ExtraHeaders)
	if err != nil {
		return Provider{}, err
	}
	now := nowString()
	var cipher string
	var exists int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM providers WHERE id = ?`, p.ID).Scan(&exists)
	if exists > 0 && p.APIKey == "" {
		if err := s.db.QueryRow(`SELECT api_key_cipher FROM providers WHERE id = ?`, p.ID).Scan(&cipher); err != nil {
			return Provider{}, err
		}
	} else {
		cipher, err = encryptSecret(s.secretKey, p.APIKey)
		if err != nil {
			return Provider{}, err
		}
	}
	if exists == 0 {
		_, err = s.db.Exec(`INSERT INTO providers(id, name, base_url, api_key_cipher, extra_headers_json, enabled, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.BaseURL, cipher, headers, boolInt(p.Enabled), now, now)
	} else {
		_, err = s.db.Exec(`UPDATE providers SET name = ?, base_url = ?, api_key_cipher = ?, extra_headers_json = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			p.Name, p.BaseURL, cipher, headers, boolInt(p.Enabled), now, p.ID)
	}
	if err != nil {
		return Provider{}, err
	}
	return s.GetProvider(p.ID, false)
}

func (s *Store) DeleteProvider(id string) error {
	_, err := s.db.Exec(`DELETE FROM providers WHERE id = ?`, id)
	return err
}

func (s *Store) UpsertModel(m Model) (Model, error) {
	if m.ProviderID == "" || m.OriginalID == "" {
		return Model{}, errors.New("provider_id 和 original_id 不能为空")
	}
	m.InternalID = internalModelID(m.ProviderID, m.OriginalID)
	if m.DisplayName == "" {
		m.DisplayName = m.OriginalID
	}
	now := nowString()
	var exists int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM models WHERE internal_id = ?`, m.InternalID).Scan(&exists)
	if exists == 0 {
		_, err := s.db.Exec(`INSERT INTO models(internal_id, provider_id, original_id, display_name, supports_chat, supports_responses, supports_stream, context_length, enabled, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.InternalID, m.ProviderID, m.OriginalID, m.DisplayName, boolInt(m.SupportsChat), boolInt(m.SupportsResponses), boolInt(m.SupportsStream), m.ContextLength, boolInt(m.Enabled), now, now)
		if err != nil {
			return Model{}, err
		}
	} else {
		_, err := s.db.Exec(`UPDATE models SET display_name = ?, supports_chat = ?, supports_responses = ?, supports_stream = ?, context_length = ?, enabled = ?, updated_at = ? WHERE internal_id = ?`,
			m.DisplayName, boolInt(m.SupportsChat), boolInt(m.SupportsResponses), boolInt(m.SupportsStream), m.ContextLength, boolInt(m.Enabled), now, m.InternalID)
		if err != nil {
			return Model{}, err
		}
	}
	return s.GetModel(m.InternalID)
}

func (s *Store) GetModel(id string) (Model, error) {
	var m Model
	var providerEnabled bool
	err := s.db.QueryRow(`SELECT m.internal_id, m.provider_id, m.original_id, m.display_name, m.supports_chat, m.supports_responses, m.supports_stream, m.context_length,
		m.enabled, m.auto_disabled, m.auto_disabled_reason, m.fail_count, m.window_start, m.last_failure_at, m.cooldown_until, m.cooldown_count,
		m.upstream_error_status, m.upstream_error_at, m.upstream_error, p.enabled
		FROM models m JOIN providers p ON p.id = m.provider_id WHERE m.internal_id = ?`, id).
		Scan(&m.InternalID, &m.ProviderID, &m.OriginalID, &m.DisplayName, &m.SupportsChat, &m.SupportsResponses, &m.SupportsStream, &m.ContextLength,
			&m.Enabled, &m.AutoDisabled, &m.AutoDisabledReason, &m.FailCount, &m.WindowStart, &m.LastFailureAt, &m.CooldownUntil, &m.CooldownCount,
			&m.UpstreamErrorStatus, &m.UpstreamErrorAt, &m.UpstreamError, &providerEnabled)
	m.ProviderEnabled = providerEnabled
	return m, err
}

func (s *Store) ListModels() ([]Model, error) {
	rows, err := s.db.Query(`SELECT m.internal_id, m.provider_id, m.original_id, m.display_name, m.supports_chat, m.supports_responses, m.supports_stream, m.context_length,
		m.enabled, m.auto_disabled, m.auto_disabled_reason, m.fail_count, m.window_start, m.last_failure_at, m.cooldown_until, m.cooldown_count,
		m.upstream_error_status, m.upstream_error_at, m.upstream_error, p.enabled
		FROM models m JOIN providers p ON p.id = m.provider_id ORDER BY m.provider_id, m.original_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := []Model{}
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.InternalID, &m.ProviderID, &m.OriginalID, &m.DisplayName, &m.SupportsChat, &m.SupportsResponses, &m.SupportsStream, &m.ContextLength,
			&m.Enabled, &m.AutoDisabled, &m.AutoDisabledReason, &m.FailCount, &m.WindowStart, &m.LastFailureAt, &m.CooldownUntil, &m.CooldownCount,
			&m.UpstreamErrorStatus, &m.UpstreamErrorAt, &m.UpstreamError, &m.ProviderEnabled); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func (s *Store) RestoreModel(id string) error {
	_, err := s.db.Exec(`UPDATE models SET auto_disabled = 0, auto_disabled_reason = '', fail_count = 0, window_start = '', last_failure_at = '', cooldown_until = '', cooldown_count = 0, upstream_error_status = 0, upstream_error_at = '', upstream_error = '', updated_at = ? WHERE internal_id = ?`, nowString(), id)
	return err
}

func (s *Store) RecordModelFailure(modelID, reason string) error {
	now := timeNow()
	var failCount int
	var cooldownCount int
	var windowStart, cooldownUntil string
	err := s.db.QueryRow(`SELECT fail_count, window_start, cooldown_until, cooldown_count FROM models WHERE internal_id = ?`, modelID).Scan(&failCount, &windowStart, &cooldownUntil, &cooldownCount)
	if err != nil {
		return err
	}
	reset := true
	if windowStart != "" {
		if started, err := parseTime(windowStart); err == nil && now.Sub(started) <= modelFailureWindow {
			reset = false
		}
	}
	if reset {
		failCount = 0
		windowStart = now.Format(timeFormat)
	}
	failCount++
	autoDisabled := false
	autoReason := ""
	if failCount >= modelFailureThreshold {
		cooldownCount++
		failCount = 0
		windowStart = ""
		cooldownUntil = now.Add(modelCooldownDuration).Format(timeFormat)
		if cooldownCount >= modelCooldownLimit {
			autoDisabled = true
			autoReason = fmt.Sprintf("连续冷却 %d 次后自动禁用。最后错误：%s", cooldownCount, reason)
			cooldownUntil = ""
		}
	}
	if autoDisabled {
		cooldownCount = modelCooldownLimit
	}
	_, err = s.db.Exec(`UPDATE models SET fail_count = ?, window_start = ?, last_failure_at = ?, auto_disabled = ?, auto_disabled_reason = ?, cooldown_until = ?, cooldown_count = ?, updated_at = ? WHERE internal_id = ?`,
		failCount, windowStart, now.Format(timeFormat), boolInt(autoDisabled), autoReason, cooldownUntil, cooldownCount, now.Format(timeFormat), modelID)
	return err
}

func (s *Store) RecordModelSuccess(modelID string) error {
	_, err := s.db.Exec(`UPDATE models SET auto_disabled = 0, auto_disabled_reason = '', fail_count = 0, window_start = '', last_failure_at = '', cooldown_until = '', cooldown_count = 0, updated_at = ? WHERE internal_id = ?`, nowString(), modelID)
	return err
}

func (s *Store) RecordModelUpstreamError(modelID string, status int, reason string) error {
	reason = strings.TrimSpace(reason)
	const maxReasonLength = 500
	runes := []rune(reason)
	if len(runes) > maxReasonLength {
		reason = string(runes[:maxReasonLength]) + "..."
	}
	now := nowString()
	_, err := s.db.Exec(`UPDATE models SET upstream_error_status = ?, upstream_error_at = ?, upstream_error = ?, updated_at = ? WHERE internal_id = ?`,
		status, now, reason, now, modelID)
	return err
}

func (s *Store) UpsertGroup(g ModelGroup) (ModelGroup, error) {
	if g.ID == "" {
		g.ID = newID("grp")
	}
	if g.Name == "" {
		return ModelGroup{}, errors.New("模型组名称不能为空")
	}
	if g.Strategy == "" {
		g.Strategy = "fallback"
	}
	now := nowString()
	tx, err := s.db.Begin()
	if err != nil {
		return ModelGroup{}, err
	}
	defer tx.Rollback()
	var exists int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM model_groups WHERE id = ?`, g.ID).Scan(&exists)
	if exists == 0 {
		if _, err := tx.Exec(`INSERT INTO model_groups(id, name, strategy, enabled, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`, g.ID, g.Name, g.Strategy, boolInt(g.Enabled), now, now); err != nil {
			return ModelGroup{}, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE model_groups SET name = ?, strategy = ?, enabled = ?, updated_at = ? WHERE id = ?`, g.Name, g.Strategy, boolInt(g.Enabled), now, g.ID); err != nil {
			return ModelGroup{}, err
		}
		if _, err := tx.Exec(`DELETE FROM model_group_members WHERE group_id = ?`, g.ID); err != nil {
			return ModelGroup{}, err
		}
	}
	for i, member := range g.Members {
		if member.Position == 0 {
			member.Position = i + 1
		}
		if _, err := tx.Exec(`INSERT INTO model_group_members(group_id, model_id, position, enabled) VALUES(?, ?, ?, ?)`, g.ID, member.ModelID, member.Position, boolInt(member.Enabled)); err != nil {
			return ModelGroup{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ModelGroup{}, err
	}
	return s.GetGroup(g.ID)
}

func (s *Store) GetGroup(id string) (ModelGroup, error) {
	var g ModelGroup
	err := s.db.QueryRow(`SELECT id, name, strategy, enabled, cursor, created_at, updated_at FROM model_groups WHERE id = ?`, id).
		Scan(&g.ID, &g.Name, &g.Strategy, &g.Enabled, &g.Cursor, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return ModelGroup{}, err
	}
	members, err := s.groupMembers(id)
	if err != nil {
		return ModelGroup{}, err
	}
	g.Members = members
	return g, nil
}

func (s *Store) ListGroups() ([]ModelGroup, error) {
	rows, err := s.db.Query(`SELECT id FROM model_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	groups := []ModelGroup{}
	for _, id := range ids {
		group, err := s.GetGroup(id)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *Store) groupMembers(groupID string) ([]ModelGroupMember, error) {
	rows, err := s.db.Query(`SELECT model_id, position, enabled FROM model_group_members WHERE group_id = ? ORDER BY position`, groupID)
	if err != nil {
		return nil, err
	}
	members := []ModelGroupMember{}
	for rows.Next() {
		var member ModelGroupMember
		if err := rows.Scan(&member.ModelID, &member.Position, &member.Enabled); err != nil {
			_ = rows.Close()
			return nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range members {
		if model, err := s.GetModel(members[i].ModelID); err == nil {
			members[i].Model = &model
		}
	}
	return members, nil
}

func (s *Store) DeleteGroup(id string) error {
	_, err := s.db.Exec(`DELETE FROM model_groups WHERE id = ?`, id)
	return err
}

func (s *Store) AdvanceGroupCursor(groupID string, size int) (int, error) {
	if size <= 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var cursor int
	if err := tx.QueryRow(`SELECT cursor FROM model_groups WHERE id = ?`, groupID).Scan(&cursor); err != nil {
		return 0, err
	}
	next := (cursor + 1) % size
	if _, err := tx.Exec(`UPDATE model_groups SET cursor = ? WHERE id = ?`, next, groupID); err != nil {
		return 0, err
	}
	return cursor % size, tx.Commit()
}

func (s *Store) UpsertRoute(r Route) (Route, error) {
	if r.ID == "" {
		return Route{}, errors.New("路由模型 ID 不能为空")
	}
	if r.Name == "" {
		r.Name = r.ID
	}
	now := nowString()
	tx, err := s.db.Begin()
	if err != nil {
		return Route{}, err
	}
	defer tx.Rollback()
	var exists int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM routes WHERE id = ?`, r.ID).Scan(&exists)
	if exists == 0 {
		if _, err := tx.Exec(`INSERT INTO routes(id, name, enabled, created_at, updated_at) VALUES(?, ?, ?, ?, ?)`, r.ID, r.Name, boolInt(r.Enabled), now, now); err != nil {
			return Route{}, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE routes SET name = ?, enabled = ?, updated_at = ? WHERE id = ?`, r.Name, boolInt(r.Enabled), now, r.ID); err != nil {
			return Route{}, err
		}
		if _, err := tx.Exec(`DELETE FROM route_steps WHERE route_id = ?`, r.ID); err != nil {
			return Route{}, err
		}
	}
	for i, step := range r.Steps {
		if step.Position == 0 {
			step.Position = i + 1
		}
		if _, err := tx.Exec(`INSERT INTO route_steps(route_id, position, target_type, target_id, enabled) VALUES(?, ?, ?, ?, ?)`, r.ID, step.Position, step.TargetType, step.TargetID, boolInt(step.Enabled)); err != nil {
			return Route{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Route{}, err
	}
	return s.GetRoute(r.ID)
}

func (s *Store) GetRoute(id string) (Route, error) {
	var r Route
	err := s.db.QueryRow(`SELECT id, name, enabled, created_at, updated_at FROM routes WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Route{}, err
	}
	steps, err := s.routeSteps(id)
	if err != nil {
		return Route{}, err
	}
	r.Steps = steps
	overrides, err := s.RouteOverrides(id)
	if err != nil {
		return Route{}, err
	}
	r.Overrides = overrides
	return r, nil
}

func (s *Store) ListRoutes() ([]Route, error) {
	rows, err := s.db.Query(`SELECT id FROM routes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	routes := []Route{}
	for _, id := range ids {
		route, err := s.GetRoute(id)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (s *Store) EnabledRoutes() ([]Route, error) {
	rows, err := s.db.Query(`SELECT id FROM routes WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	routes := []Route{}
	for _, id := range ids {
		route, err := s.GetRoute(id)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (s *Store) routeSteps(routeID string) ([]RouteStep, error) {
	rows, err := s.db.Query(`SELECT id, position, target_type, target_id, enabled FROM route_steps WHERE route_id = ? ORDER BY position`, routeID)
	if err != nil {
		return nil, err
	}
	steps := []RouteStep{}
	for rows.Next() {
		var step RouteStep
		if err := rows.Scan(&step.ID, &step.Position, &step.TargetType, &step.TargetID, &step.Enabled); err != nil {
			_ = rows.Close()
			return nil, err
		}
		step.Label = step.TargetID
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range steps {
		if steps[i].TargetType == "group" {
			if group, err := s.GetGroup(steps[i].TargetID); err == nil {
				steps[i].Label = group.Name
			}
		}
	}
	return steps, nil
}

func (s *Store) SetRouteEnabled(id string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE routes SET enabled = ?, updated_at = ? WHERE id = ?`, boolInt(enabled), nowString(), id)
	return err
}

func (s *Store) DeleteRoute(id string) error {
	_, err := s.db.Exec(`DELETE FROM routes WHERE id = ?`, id)
	return err
}

func (s *Store) SetRouteOverride(o Override) error {
	if !o.Disabled {
		_, err := s.db.Exec(`DELETE FROM route_overrides WHERE route_id = ? AND target_type = ? AND target_id = ?`, o.RouteID, o.TargetType, o.TargetID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO route_overrides(route_id, target_type, target_id, disabled) VALUES(?, ?, ?, ?)
		ON CONFLICT(route_id, target_type, target_id) DO UPDATE SET disabled = excluded.disabled`,
		o.RouteID, o.TargetType, o.TargetID, boolInt(o.Disabled))
	return err
}

func (s *Store) RouteOverrides(routeID string) ([]Override, error) {
	rows, err := s.db.Query(`SELECT route_id, target_type, target_id, disabled FROM route_overrides WHERE route_id = ?`, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	overrides := []Override{}
	for rows.Next() {
		var o Override
		if err := rows.Scan(&o.RouteID, &o.TargetType, &o.TargetID, &o.Disabled); err != nil {
			return nil, err
		}
		overrides = append(overrides, o)
	}
	return overrides, rows.Err()
}

func (s *Store) AddRequestLog(log RequestLog) error {
	if log.ID == "" {
		log.ID = newID("req")
	}
	if log.CreatedAt == "" {
		log.CreatedAt = nowString()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO request_logs(id, created_at, api, route_id, client_model, final_model, status, http_status, duration_ms, prompt_tokens, completion_tokens, total_tokens, error)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.CreatedAt, log.API, log.RouteID, log.ClientModel, log.FinalModel, log.Status, log.HTTPStatus, log.DurationMS, log.PromptTokens, log.CompletionTokens, log.TotalTokens, log.Error)
	if err != nil {
		return err
	}
	for i, attempt := range log.Attempts {
		if attempt.Position == 0 {
			attempt.Position = i + 1
		}
		if _, err := tx.Exec(`INSERT INTO attempt_logs(request_id, position, model_id, provider_id, status, http_status, duration_ms, error) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			log.ID, attempt.Position, attempt.ModelID, attempt.ProviderID, attempt.Status, attempt.HTTPStatus, attempt.DurationMS, attempt.Error); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListLogs(limit int) ([]RequestLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, created_at, api, route_id, client_model, final_model, status, http_status, duration_ms, prompt_tokens, completion_tokens, total_tokens, error
		FROM request_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	logs := []RequestLog{}
	for rows.Next() {
		var item RequestLog
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.API, &item.RouteID, &item.ClientModel, &item.FinalModel, &item.Status, &item.HTTPStatus, &item.DurationMS, &item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.Error); err != nil {
			_ = rows.Close()
			return nil, err
		}
		logs = append(logs, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range logs {
		attempts, err := s.AttemptsForLog(logs[i].ID)
		if err != nil {
			return nil, err
		}
		logs[i].Attempts = attempts
	}
	return logs, nil
}

func (s *Store) AttemptsForLog(requestID string) ([]AttemptLog, error) {
	rows, err := s.db.Query(`SELECT id, request_id, position, model_id, provider_id, status, http_status, duration_ms, error FROM attempt_logs WHERE request_id = ? ORDER BY position`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := []AttemptLog{}
	for rows.Next() {
		var attempt AttemptLog
		if err := rows.Scan(&attempt.ID, &attempt.RequestID, &attempt.Position, &attempt.ModelID, &attempt.ProviderID, &attempt.Status, &attempt.HTTPStatus, &attempt.DurationMS, &attempt.Error); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *Store) CleanupLogs(days int) error {
	if days <= 0 {
		return nil
	}
	cutoff := timeNow().Add(-time.Duration(days) * 24 * time.Hour).Format(timeFormat)
	_, err := s.db.Exec(`DELETE FROM request_logs WHERE created_at < ?`, cutoff)
	return err
}

func (s *Store) ImportProviderModels(providerID string, models []Model) ([]Model, error) {
	out := make([]Model, 0, len(models))
	for _, model := range models {
		model.ProviderID = providerID
		if !model.SupportsChat && !model.SupportsResponses {
			model.SupportsChat = true
		}
		if !model.Enabled {
			model.Enabled = true
		}
		saved, err := s.UpsertModel(model)
		if err != nil {
			return nil, err
		}
		out = append(out, saved)
	}
	return out, nil
}

func (s *Store) LoadProviderForModel(modelID string) (Provider, Model, error) {
	model, err := s.GetModel(modelID)
	if err != nil {
		return Provider{}, Model{}, err
	}
	provider, err := s.GetProvider(model.ProviderID, true)
	if err != nil {
		return Provider{}, Model{}, err
	}
	return provider, model, nil
}

func (s *Store) RawModelsForModelsEndpoint() ([]Model, error) {
	value, err := s.GetSetting("models_expose_raw")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if value != "true" {
		return nil, nil
	}
	autoDisableEnabled, err := s.AutoDisableModelsEnabled()
	if err != nil {
		return nil, err
	}
	models, err := s.ListModels()
	if err != nil {
		return nil, err
	}
	filtered := make([]Model, 0, len(models))
	for _, model := range models {
		if model.Enabled && model.ProviderEnabled && (!autoDisableEnabled || (!model.AutoDisabled && !model.CoolingDown())) {
			filtered = append(filtered, model)
		}
	}
	return filtered, nil
}

func (s *Store) EncodeJSON(value any) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}

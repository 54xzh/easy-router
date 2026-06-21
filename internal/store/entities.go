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
	rows, err := s.db.Query(`SELECT id, name, base_url, api_key_cipher, extra_headers_json, enabled, parent_id, multi_key_enabled, multi_key_strategy, key_name, key_prefix, key_position, created_at, updated_at FROM providers WHERE parent_id = '' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	providers := []Provider{}
	for rows.Next() {
		p, err := scanProvider(rows, false, s.secretKey)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		providers = append(providers, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range providers {
		if err := s.fillProviderKeyCounts(&providers[i]); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func (s *Store) GetProvider(id string, includeKey bool) (Provider, error) {
	row := s.db.QueryRow(`SELECT id, name, base_url, api_key_cipher, extra_headers_json, enabled, parent_id, multi_key_enabled, multi_key_strategy, key_name, key_prefix, key_position, created_at, updated_at FROM providers WHERE id = ?`, id)
	p, err := scanProvider(row, includeKey, s.secretKey)
	if err != nil {
		return Provider{}, err
	}
	if p.ParentID == "" {
		if err := s.fillProviderKeyCounts(&p); err != nil {
			return Provider{}, err
		}
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
	if p.MultiKeyStrategy == "" {
		p.MultiKeyStrategy = "round_robin"
	}
	headers, err := marshalHeaders(p.ExtraHeaders)
	if err != nil {
		return Provider{}, err
	}
	now := nowString()
	var cipher string
	var exists int
	var old Provider
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM providers WHERE id = ?`, p.ID).Scan(&exists)
	if exists > 0 {
		old, _ = s.GetProvider(p.ID, false)
	}
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
		_, err = s.db.Exec(`INSERT INTO providers(id, name, base_url, api_key_cipher, extra_headers_json, enabled, parent_id, multi_key_enabled, multi_key_strategy, key_name, key_prefix, key_position, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.BaseURL, cipher, headers, boolInt(p.Enabled), p.ParentID, boolInt(p.MultiKeyEnabled), p.MultiKeyStrategy, p.KeyName, p.KeyPrefix, p.KeyPosition, now, now)
	} else {
		_, err = s.db.Exec(`UPDATE providers SET name = ?, base_url = ?, api_key_cipher = ?, extra_headers_json = ?, enabled = ?, parent_id = ?, multi_key_enabled = ?, multi_key_strategy = ?, key_name = ?, key_prefix = ?, key_position = ?, updated_at = ? WHERE id = ?`,
			p.Name, p.BaseURL, cipher, headers, boolInt(p.Enabled), p.ParentID, boolInt(p.MultiKeyEnabled), p.MultiKeyStrategy, p.KeyName, p.KeyPrefix, p.KeyPosition, now, p.ID)
	}
	if err != nil {
		return Provider{}, err
	}
	if p.ParentID == "" {
		if p.MultiKeyEnabled {
			if err := s.ensureProviderHasInitialKey(p.ID); err != nil {
				return Provider{}, err
			}
		} else if old.MultiKeyEnabled && p.APIKey == "" {
			if err := s.copyFirstEnabledKeyToProvider(p.ID); err != nil {
				return Provider{}, err
			}
		}
		if err := s.syncProviderConfigToKeys(p.ID); err != nil {
			return Provider{}, err
		}
	}
	return s.GetProvider(p.ID, false)
}

func scanProvider(row interface{ Scan(dest ...any) error }, includeKey bool, secretKey string) (Provider, error) {
	var p Provider
	var headers, cipher string
	err := row.Scan(&p.ID, &p.Name, &p.BaseURL, &cipher, &headers, &p.Enabled, &p.ParentID, &p.MultiKeyEnabled, &p.MultiKeyStrategy, &p.KeyName, &p.KeyPrefix, &p.KeyPosition, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Provider{}, err
	}
	p.ExtraHeaders = unmarshalHeaders(headers)
	if p.MultiKeyStrategy == "" {
		p.MultiKeyStrategy = "round_robin"
	}
	if includeKey {
		key, err := decryptSecret(secretKey, cipher)
		if err != nil {
			return Provider{}, err
		}
		p.APIKey = key
	}
	return p, nil
}

func (s *Store) fillProviderKeyCounts(p *Provider) error {
	if p.ParentID != "" {
		return nil
	}
	return s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(enabled), 0) FROM providers WHERE parent_id = ?`, p.ID).Scan(&p.KeyCount, &p.EnabledKeyCount)
}

func (s *Store) ensureProviderHasInitialKey(providerID string) error {
	keys, err := s.ListProviderKeys(providerID)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return nil
	}
	provider, err := s.GetProvider(providerID, true)
	if err != nil {
		return err
	}
	if provider.APIKey == "" {
		return nil
	}
	_, err = s.AddProviderKey(providerID, ProviderKey{
		Name:   "Key 1",
		APIKey: provider.APIKey,
	})
	return err
}

func (s *Store) copyFirstEnabledKeyToProvider(providerID string) error {
	keys, err := s.ListProviderKeys(providerID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if !key.Enabled {
			continue
		}
		child, err := s.GetProvider(key.ID, true)
		if err != nil {
			return err
		}
		cipher, err := encryptSecret(s.secretKey, child.APIKey)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(`UPDATE providers SET api_key_cipher = ?, updated_at = ? WHERE id = ?`, cipher, nowString(), providerID)
		return err
	}
	return nil
}

func (s *Store) syncProviderConfigToKeys(providerID string) error {
	provider, err := s.GetProvider(providerID, false)
	if err != nil {
		return err
	}
	headers, err := marshalHeaders(provider.ExtraHeaders)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE providers SET base_url = ?, extra_headers_json = ?, updated_at = ? WHERE parent_id = ?`, provider.BaseURL, headers, nowString(), providerID)
	return err
}

func (s *Store) DeleteProvider(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM providers WHERE parent_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM providers WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListProviderKeys(providerID string) ([]ProviderKey, error) {
	rows, err := s.db.Query(`SELECT id, name, base_url, api_key_cipher, extra_headers_json, enabled, parent_id, multi_key_enabled, multi_key_strategy, key_name, key_prefix, key_position, created_at, updated_at FROM providers WHERE parent_id = ? ORDER BY key_position, created_at`, providerID)
	if err != nil {
		return nil, err
	}
	keys := []ProviderKey{}
	for rows.Next() {
		provider, err := scanProvider(rows, false, s.secretKey)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, providerKeyFromProvider(provider))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range keys {
		if err := s.fillProviderKeyIssueCount(&keys[i]); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (s *Store) AddProviderKey(providerID string, key ProviderKey) (ProviderKey, error) {
	parent, err := s.GetProvider(providerID, false)
	if err != nil {
		return ProviderKey{}, err
	}
	if parent.ParentID != "" {
		return ProviderKey{}, errors.New("只能给主提供商添加 Key")
	}
	if strings.TrimSpace(key.APIKey) == "" {
		return ProviderKey{}, errors.New("API Key 不能为空")
	}
	position, err := s.nextProviderKeyPosition(providerID)
	if err != nil {
		return ProviderKey{}, err
	}
	if key.Name == "" {
		key.Name = fmt.Sprintf("Key %d", position)
	}
	key.ID = newID("kp")
	key.ProviderID = providerID
	key.Prefix = tokenPrefix(key.APIKey)
	key.Position = position
	key.Enabled = true
	cipher, err := encryptSecret(s.secretKey, key.APIKey)
	if err != nil {
		return ProviderKey{}, err
	}
	headers, err := marshalHeaders(parent.ExtraHeaders)
	if err != nil {
		return ProviderKey{}, err
	}
	now := nowString()
	if _, err := s.db.Exec(`INSERT INTO providers(id, name, base_url, api_key_cipher, extra_headers_json, enabled, parent_id, multi_key_enabled, multi_key_strategy, key_name, key_prefix, key_position, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, 0, '', ?, ?, ?, ?, ?)`,
		key.ID, key.Name, parent.BaseURL, cipher, headers, boolInt(key.Enabled), providerID, key.Name, key.Prefix, key.Position, now, now); err != nil {
		return ProviderKey{}, err
	}
	if err := s.syncProviderModelsToKey(providerID, key.ID); err != nil {
		return ProviderKey{}, err
	}
	return s.GetProviderKey(providerID, key.ID)
}

func (s *Store) GetProviderKey(providerID, keyID string) (ProviderKey, error) {
	provider, err := s.GetProvider(keyID, false)
	if err != nil {
		return ProviderKey{}, err
	}
	if provider.ParentID != providerID {
		return ProviderKey{}, sql.ErrNoRows
	}
	key := providerKeyFromProvider(provider)
	return key, s.fillProviderKeyIssueCount(&key)
}

func (s *Store) UpdateProviderKey(providerID string, key ProviderKey) (ProviderKey, error) {
	current, err := s.GetProviderKey(providerID, key.ID)
	if err != nil {
		return ProviderKey{}, err
	}
	if key.Name == "" {
		key.Name = current.Name
	}
	_, err = s.db.Exec(`UPDATE providers SET name = ?, key_name = ?, enabled = ?, updated_at = ? WHERE id = ? AND parent_id = ?`, key.Name, key.Name, boolInt(key.Enabled), nowString(), key.ID, providerID)
	if err != nil {
		return ProviderKey{}, err
	}
	return s.GetProviderKey(providerID, key.ID)
}

func (s *Store) RotateProviderKey(providerID, keyID, apiKey string) (ProviderKey, error) {
	if strings.TrimSpace(apiKey) == "" {
		return ProviderKey{}, errors.New("API Key 不能为空")
	}
	if _, err := s.GetProviderKey(providerID, keyID); err != nil {
		return ProviderKey{}, err
	}
	cipher, err := encryptSecret(s.secretKey, apiKey)
	if err != nil {
		return ProviderKey{}, err
	}
	prefix := tokenPrefix(apiKey)
	if _, err := s.db.Exec(`UPDATE providers SET api_key_cipher = ?, key_prefix = ?, updated_at = ? WHERE id = ? AND parent_id = ?`, cipher, prefix, nowString(), keyID, providerID); err != nil {
		return ProviderKey{}, err
	}
	return s.GetProviderKey(providerID, keyID)
}

func (s *Store) DeleteProviderKey(providerID, keyID string) error {
	_, err := s.db.Exec(`DELETE FROM providers WHERE id = ? AND parent_id = ?`, keyID, providerID)
	return err
}

func (s *Store) SetProviderKeyOrder(providerID string, ids []string) ([]ProviderKey, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for index, id := range ids {
		if _, err := tx.Exec(`UPDATE providers SET key_position = ?, updated_at = ? WHERE id = ? AND parent_id = ?`, index+1, nowString(), id, providerID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListProviderKeys(providerID)
}

func (s *Store) RestoreProviderKeyModels(providerID, keyID string) error {
	if _, err := s.GetProviderKey(providerID, keyID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE models SET auto_disabled = 0, auto_disabled_reason = '', fail_count = 0, window_start = '', last_failure_at = '', cooldown_until = '', cooldown_count = 0, upstream_error_status = 0, upstream_error_at = '', upstream_error = '', updated_at = ? WHERE provider_id = ?`, nowString(), keyID)
	return err
}

func (s *Store) fillProviderKeyIssueCount(key *ProviderKey) error {
	return s.db.QueryRow(`SELECT COUNT(*) FROM models WHERE provider_id = ? AND (auto_disabled = 1 OR cooldown_until > ?)`, key.ID, nowString()).Scan(&key.ModelIssueCount)
}

func (s *Store) nextProviderKeyPosition(providerID string) (int, error) {
	var position int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(key_position), 0) + 1 FROM providers WHERE parent_id = ?`, providerID).Scan(&position)
	return position, err
}

func providerKeyFromProvider(provider Provider) ProviderKey {
	name := provider.KeyName
	if name == "" {
		name = provider.Name
	}
	return ProviderKey{
		ID:         provider.ID,
		ProviderID: provider.ParentID,
		Name:       name,
		Prefix:     provider.KeyPrefix,
		Enabled:    provider.Enabled,
		Position:   provider.KeyPosition,
		CreatedAt:  provider.CreatedAt,
		UpdatedAt:  provider.UpdatedAt,
	}
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
	saved, err := s.GetModel(m.InternalID)
	if err != nil {
		return Model{}, err
	}
	provider, err := s.GetProvider(saved.ProviderID, false)
	if err == nil && provider.ParentID == "" {
		if err := s.syncProviderModelToKeys(saved); err != nil {
			return Model{}, err
		}
	}
	return saved, nil
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
		FROM models m JOIN providers p ON p.id = m.provider_id WHERE p.parent_id = '' ORDER BY m.provider_id, m.original_id`)
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

func (s *Store) syncProviderModelsToKey(providerID, keyID string) error {
	rows, err := s.db.Query(`SELECT internal_id FROM models WHERE provider_id = ? ORDER BY original_id`, providerID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		model, err := s.GetModel(id)
		if err != nil {
			return err
		}
		model.ProviderID = keyID
		if _, err := s.UpsertModel(model); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) syncProviderModelToKeys(model Model) error {
	keys, err := s.ListProviderKeys(model.ProviderID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		copy := model
		copy.ProviderID = key.ID
		if _, err := s.UpsertModel(copy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListProviderKeyModels(providerID, originalID string) ([]ProviderKeyModel, error) {
	keys, err := s.ListProviderKeys(providerID)
	if err != nil {
		return nil, err
	}
	items := []ProviderKeyModel{}
	for _, key := range keys {
		if !key.Enabled {
			continue
		}
		model, err := s.GetModel(internalModelID(key.ID, originalID))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		items = append(items, ProviderKeyModel{Key: key, Model: model})
	}
	return items, nil
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

func (s *Store) AdvanceMultiKeyCursor(providerID, originalID string, size int) (int, error) {
	if size <= 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO multi_key_model_cursors(provider_id, original_id, cursor) VALUES(?, ?, 0)`, providerID, originalID); err != nil {
		return 0, err
	}
	var cursor int
	if err := tx.QueryRow(`SELECT cursor FROM multi_key_model_cursors WHERE provider_id = ? AND original_id = ?`, providerID, originalID).Scan(&cursor); err != nil {
		return 0, err
	}
	next := (cursor + 1) % size
	if _, err := tx.Exec(`UPDATE multi_key_model_cursors SET cursor = ? WHERE provider_id = ? AND original_id = ?`, next, providerID, originalID); err != nil {
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
	_, err = tx.Exec(`INSERT INTO request_logs(id, created_at, api, route_id, client_model, final_model, status, http_status, duration_ms, first_token_ms, prompt_tokens, completion_tokens, total_tokens, error)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.CreatedAt, log.API, log.RouteID, log.ClientModel, log.FinalModel, log.Status, log.HTTPStatus, log.DurationMS, log.FirstTokenMS, log.PromptTokens, log.CompletionTokens, log.TotalTokens, log.Error)
	if err != nil {
		return err
	}
	for i, attempt := range log.Attempts {
		if attempt.Position == 0 {
			attempt.Position = i + 1
		}
		if _, err := tx.Exec(`INSERT INTO attempt_logs(request_id, position, model_id, provider_id, key_name, key_prefix, status, http_status, duration_ms, first_token_ms, error) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			log.ID, attempt.Position, attempt.ModelID, attempt.ProviderID, attempt.KeyName, attempt.KeyPrefix, attempt.Status, attempt.HTTPStatus, attempt.DurationMS, attempt.FirstTokenMS, attempt.Error); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListLogs(limit int) ([]RequestLog, error) {
	page, err := s.ListLogsFiltered(LogFilter{Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Store) ListLogsFiltered(filter LogFilter) (LogPage, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	var (
		where  []string
		args   []any
		cursor []string
	)
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.ClientModel != "" {
		where = append(where, "client_model = ?")
		args = append(args, filter.ClientModel)
	}
	if filter.Q != "" {
		where = append(where, "(error LIKE ? OR client_model LIKE ? OR final_model LIKE ?)")
		pattern := "%" + filter.Q + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if filter.After != "" {
		where = append(where, "created_at >= ?")
		args = append(args, filter.After)
	}
	if filter.Before != "" {
		where = append(where, "created_at <= ?")
		args = append(args, filter.Before)
	}
	if filter.CursorCreatedAt != "" && filter.CursorID != "" {
		cursor = append(cursor, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, filter.CursorCreatedAt, filter.CursorCreatedAt, filter.CursorID)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
		if len(cursor) > 0 {
			whereSQL += " AND " + strings.Join(cursor, " AND ")
		}
	} else if len(cursor) > 0 {
		whereSQL = " WHERE " + strings.Join(cursor, " AND ")
	}

	query := `SELECT id, created_at, api, route_id, client_model, final_model, status, http_status, duration_ms, first_token_ms, prompt_tokens, completion_tokens, total_tokens, error
		FROM request_logs` + whereSQL + ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, filter.Limit+1)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return LogPage{}, err
	}
	logs := []RequestLog{}
	for rows.Next() {
		var item RequestLog
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.API, &item.RouteID, &item.ClientModel, &item.FinalModel, &item.Status, &item.HTTPStatus, &item.DurationMS, &item.FirstTokenMS, &item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.Error); err != nil {
			_ = rows.Close()
			return LogPage{}, err
		}
		logs = append(logs, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return LogPage{}, err
	}
	if err := rows.Close(); err != nil {
		return LogPage{}, err
	}

	var nextCursor *LogCursor
	if len(logs) > filter.Limit {
		last := logs[filter.Limit-1]
		nextCursor = &LogCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		logs = logs[:filter.Limit]
	}

	if len(logs) > 0 {
		ids := make([]string, len(logs))
		for i := range logs {
			ids[i] = logs[i].ID
		}
		attemptsByReq, err := s.AttemptsForLogs(ids)
		if err != nil {
			return LogPage{}, err
		}
		for i := range logs {
			logs[i].Attempts = attemptsByReq[logs[i].ID]
		}
	}

	return LogPage{Items: logs, NextCursor: nextCursor}, nil
}

func (s *Store) AttemptsForLog(requestID string) ([]AttemptLog, error) {
	rows, err := s.db.Query(`SELECT id, request_id, position, model_id, provider_id, key_name, key_prefix, status, http_status, duration_ms, first_token_ms, error FROM attempt_logs WHERE request_id = ? ORDER BY position`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := []AttemptLog{}
	for rows.Next() {
		var attempt AttemptLog
		if err := rows.Scan(&attempt.ID, &attempt.RequestID, &attempt.Position, &attempt.ModelID, &attempt.ProviderID, &attempt.KeyName, &attempt.KeyPrefix, &attempt.Status, &attempt.HTTPStatus, &attempt.DurationMS, &attempt.FirstTokenMS, &attempt.Error); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *Store) AttemptsForLogs(requestIDs []string) (map[string][]AttemptLog, error) {
	result := make(map[string][]AttemptLog, len(requestIDs))
	if len(requestIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(requestIDs))
	args := make([]any, len(requestIDs))
	for i, id := range requestIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT id, request_id, position, model_id, provider_id, key_name, key_prefix, status, http_status, duration_ms, first_token_ms, error FROM attempt_logs WHERE request_id IN (` + strings.Join(placeholders, ", ") + `) ORDER BY position`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var attempt AttemptLog
		if err := rows.Scan(&attempt.ID, &attempt.RequestID, &attempt.Position, &attempt.ModelID, &attempt.ProviderID, &attempt.KeyName, &attempt.KeyPrefix, &attempt.Status, &attempt.HTTPStatus, &attempt.DurationMS, &attempt.FirstTokenMS, &attempt.Error); err != nil {
			return nil, err
		}
		result[attempt.RequestID] = append(result[attempt.RequestID], attempt)
	}
	return result, rows.Err()
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

func (s *Store) ProviderForModelDiscovery(providerID string) (Provider, error) {
	provider, err := s.GetProvider(providerID, false)
	if err != nil {
		return Provider{}, err
	}
	if !provider.MultiKeyEnabled {
		return s.GetProvider(providerID, true)
	}
	keys, err := s.ListProviderKeys(providerID)
	if err != nil {
		return Provider{}, err
	}
	for _, key := range keys {
		if !key.Enabled {
			continue
		}
		return s.GetProvider(key.ID, true)
	}
	return Provider{}, errors.New("请先启用至少一个 Key")
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
		provider, err := s.GetProvider(model.ProviderID, false)
		if err != nil {
			return nil, err
		}
		if model.Enabled && model.ProviderEnabled && (!autoDisableEnabled || provider.MultiKeyEnabled || (!model.AutoDisabled && !model.CoolingDown())) {
			filtered = append(filtered, model)
		}
	}
	return filtered, nil
}

func (s *Store) EncodeJSON(value any) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}

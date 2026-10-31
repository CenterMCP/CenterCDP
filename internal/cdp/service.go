package cdp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Service struct {
	manager *Manager
	logger  *zap.SugaredLogger
}

func NewService(manager *Manager, logger *zap.SugaredLogger) *Service {
	return &Service{manager: manager, logger: logger}
}

func (s *Service) Manager() *Manager { return s.manager }

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

func validateKey(kind, value string) error {
	if !keyPattern.MatchString(value) {
		return fmt.Errorf("%s must start with a lowercase letter and contain only lowercase letters, numbers, _, ., or -", kind)
	}
	return nil
}

var allowedTypes = map[string]bool{
	"string": true, "integer": true, "number": true, "boolean": true,
	"date": true, "datetime": true, "enum": true, "object": true, "array": true,
}

func nowUTC() string             { return time.Now().UTC().Format(time.RFC3339Nano) }
func newID(prefix string) string { return prefix + strings.ReplaceAll(uuid.NewString(), "-", "") }

func (s *Service) DefineContactAttribute(ctx context.Context, in DefineContactAttributeInput) (map[string]any, error) {
	if err := validateKey("attribute_key", in.AttributeKey); err != nil {
		return nil, err
	}
	if !allowedTypes[in.DataType] {
		return nil, fmt.Errorf("unsupported data type %q", in.DataType)
	}
	if strings.TrimSpace(in.Description) == "" {
		return nil, errors.New("description is required")
	}
	if in.DataType == "enum" && len(in.EnumValues) == 0 {
		return nil, errors.New("enum_values are required for enum attributes")
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	enums, _ := json.Marshal(in.EnumValues)
	now := nowUTC()
	id := newID("attr_")
	_, err = db.ExecContext(ctx, `INSERT INTO contact_attribute_definitions
		(id,attribute_key,display_name,description,data_type,canonical_field,is_required,is_filterable,is_searchable,is_pii,enum_values_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(attribute_key) DO UPDATE SET display_name=excluded.display_name,description=excluded.description,
		data_type=excluded.data_type,canonical_field=excluded.canonical_field,is_required=excluded.is_required,
		is_filterable=excluded.is_filterable,is_searchable=excluded.is_searchable,is_pii=excluded.is_pii,
		enum_values_json=excluded.enum_values_json,version=version+1,updated_at=excluded.updated_at`,
		id, in.AttributeKey, in.DisplayName, in.Description, in.DataType, nullable(in.CanonicalField), in.Required,
		in.Filterable, in.Searchable, in.PII, string(enums), now, now)
	if err != nil {
		return nil, fmt.Errorf("define contact attribute: %w", err)
	}
	return map[string]any{"database_id": in.DatabaseID, "attribute_key": in.AttributeKey, "status": "active"}, nil
}

func (s *Service) DefineEvent(ctx context.Context, in DefineEventInput) (map[string]any, error) {
	if err := validateKey("event_key", in.EventKey); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Description) == "" {
		return nil, errors.New("description is required")
	}
	seen := map[string]bool{}
	for _, p := range in.Parameters {
		if err := validateKey("parameter_key", p.ParameterKey); err != nil {
			return nil, err
		}
		if seen[p.ParameterKey] {
			return nil, fmt.Errorf("duplicate event parameter %q", p.ParameterKey)
		}
		seen[p.ParameterKey] = true
		if !allowedTypes[p.DataType] {
			return nil, fmt.Errorf("unsupported data type %q for %s", p.DataType, p.ParameterKey)
		}
		if p.DataType == "enum" && len(p.EnumValues) == 0 {
			return nil, fmt.Errorf("enum_values are required for %s", p.ParameterKey)
		}
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := nowUTC()
	eventID := newID("evtdef_")
	_, err = tx.ExecContext(ctx, `INSERT INTO event_definitions
		(id,event_key,display_name,description,canonical_event,allow_extra_parameters,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(event_key) DO UPDATE SET display_name=excluded.display_name,
		description=excluded.description,canonical_event=excluded.canonical_event,
		allow_extra_parameters=excluded.allow_extra_parameters,version=version+1,updated_at=excluded.updated_at`,
		eventID, in.EventKey, in.DisplayName, in.Description, nullable(in.CanonicalEvent), in.AllowExtraParameters, now, now)
	if err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM event_definitions WHERE event_key=?`, in.EventKey).Scan(&eventID); err != nil {
		return nil, err
	}
	for _, p := range in.Parameters {
		enums, _ := json.Marshal(p.EnumValues)
		_, err = tx.ExecContext(ctx, `INSERT INTO event_parameter_definitions
			(id,event_definition_id,parameter_key,display_name,description,data_type,canonical_field,is_required,is_filterable,is_pii,enum_values_json)
			VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(event_definition_id,parameter_key) DO UPDATE SET
			display_name=excluded.display_name,description=excluded.description,data_type=excluded.data_type,
			canonical_field=excluded.canonical_field,is_required=excluded.is_required,is_filterable=excluded.is_filterable,
			is_pii=excluded.is_pii,enum_values_json=excluded.enum_values_json,version=version+1`,
			newID("param_"), eventID, p.ParameterKey, p.DisplayName, p.Description, p.DataType, nullable(p.CanonicalField), p.Required, p.Filterable, p.PII, string(enums))
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"database_id": in.DatabaseID, "event_key": in.EventKey, "parameters": len(in.Parameters)}, nil
}

func (s *Service) DataDictionary(ctx context.Context, databaseID string) (map[string]any, error) {
	db, err := s.manager.Get(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	attrs, err := queryMaps(ctx, db, `SELECT attribute_key,display_name,description,data_type,canonical_field,
		is_required,is_filterable,is_searchable,is_pii,enum_values_json,status,version
		FROM contact_attribute_definitions ORDER BY attribute_key`)
	if err != nil {
		return nil, err
	}
	events, err := queryMaps(ctx, db, `SELECT event_key,display_name,description,canonical_event,
		allow_extra_parameters,status,version FROM event_definitions ORDER BY event_key`)
	if err != nil {
		return nil, err
	}
	params, err := queryMaps(ctx, db, `SELECT e.event_key,p.parameter_key,p.display_name,p.description,p.data_type,
		p.canonical_field,p.is_required,p.is_filterable,p.is_pii,p.enum_values_json,p.status,p.version
		FROM event_parameter_definitions p JOIN event_definitions e ON e.id=p.event_definition_id
		ORDER BY e.event_key,p.parameter_key`)
	if err != nil {
		return nil, err
	}
	return map[string]any{"database_id": databaseID, "contact_attributes": attrs, "events": events, "event_parameters": params}, nil
}

func (s *Service) UpsertContact(ctx context.Context, in UpsertContactInput) (*ContactResult, error) {
	if len(in.Identities) == 0 && in.ContactID == "" {
		return nil, errors.New("at least one identity or contact_id is required")
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	if err := validateAttributes(ctx, db, in.Attributes); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	contactID := in.ContactID
	for _, identity := range in.Identities {
		normalized, err := normalizeIdentity(identity.Type, identity.Value)
		if err != nil {
			return nil, err
		}
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT contact_id FROM contact_identities WHERE identity_type=? AND normalized_value=?`, identity.Type, normalized).Scan(&existing)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if existing != "" {
			if contactID != "" && contactID != existing {
				return nil, fmt.Errorf("identity %s belongs to another contact", identity.Type)
			}
			contactID = existing
		}
	}
	created := false
	existsContact := false
	current := map[string]any{}
	if contactID != "" {
		var raw string
		err = tx.QueryRowContext(ctx, `SELECT attributes_json FROM contacts WHERE id=?`, contactID).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			existsContact = true
			_ = json.Unmarshal([]byte(raw), &current)
		}
	}
	if contactID == "" {
		contactID = newID("contact_")
	}
	for k, v := range in.Attributes {
		current[k] = v
	}
	if !existsContact {
		if err := validateRequiredAttributes(ctx, tx, current); err != nil {
			return nil, err
		}
	}
	rawAttrs, _ := json.Marshal(current)
	now := nowUTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO contacts(id,attributes_json,created_at,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET attributes_json=excluded.attributes_json,updated_at=excluded.updated_at`, contactID, string(rawAttrs), now, now)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		created = !existsContact
	}
	for _, identity := range in.Identities {
		normalized, _ := normalizeIdentity(identity.Type, identity.Value)
		_, err = tx.ExecContext(ctx, `INSERT INTO contact_identities
			(id,contact_id,identity_type,normalized_value,original_value,is_primary,created_at)
			VALUES(?,?,?,?,?,?,?) ON CONFLICT(identity_type,normalized_value) DO UPDATE SET
			original_value=excluded.original_value,is_primary=excluded.is_primary`, newID("ident_"), contactID,
			identity.Type, normalized, strings.TrimSpace(identity.Value), identity.IsPrimary, now)
		if err != nil {
			return nil, err
		}
	}
	for key, granted := range in.Consents {
		if err := validateKey("consent key", key); err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO contact_consents(contact_id,consent_key,granted,source,updated_at)
			VALUES(?,?,?,?,?) ON CONFLICT(contact_id,consent_key) DO UPDATE SET granted=excluded.granted,
			source=excluded.source,updated_at=excluded.updated_at`, contactID, key, granted, nullable(in.ConsentSource), now)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if created {
		_ = s.enrollTriggers(ctx, db, contactID, "contact_created", "")
	}
	return &ContactResult{DatabaseID: in.DatabaseID, ContactID: contactID, Created: created, Attributes: current}, nil
}

func (s *Service) GetContact(ctx context.Context, in GetContactInput) (map[string]any, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	var raw, createdAt, updatedAt string
	if err := db.QueryRowContext(ctx, `SELECT attributes_json,created_at,updated_at FROM contacts WHERE id=?`, in.ContactID).Scan(&raw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contact %q not found", in.ContactID)
		}
		return nil, err
	}
	var attributes map[string]any
	_ = json.Unmarshal([]byte(raw), &attributes)
	identities, err := queryMaps(ctx, db, `SELECT identity_type,original_value,is_primary,created_at FROM contact_identities WHERE contact_id=? ORDER BY is_primary DESC,created_at`, in.ContactID)
	if err != nil {
		return nil, err
	}
	consents, err := queryMaps(ctx, db, `SELECT consent_key,granted,source,updated_at FROM contact_consents WHERE contact_id=? ORDER BY consent_key`, in.ContactID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"database_id": in.DatabaseID, "contact_id": in.ContactID, "attributes": attributes, "identities": identities, "consents": consents, "created_at": createdAt, "updated_at": updatedAt}, nil
}

func (s *Service) SearchContacts(ctx context.Context, in SearchContactsInput) ([]map[string]any, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	var typ string
	var filterable bool
	var enumRaw sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT data_type,is_filterable,enum_values_json FROM contact_attribute_definitions WHERE attribute_key=? AND status='active'`, in.AttributeKey).Scan(&typ, &filterable, &enumRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contact attribute %q is not defined", in.AttributeKey)
		}
		return nil, err
	}
	if !filterable {
		return nil, fmt.Errorf("contact attribute %q is not filterable", in.AttributeKey)
	}
	var enums []string
	if enumRaw.Valid {
		_ = json.Unmarshal([]byte(enumRaw.String), &enums)
	}
	if err := validateTypedValue(in.AttributeKey, typ, enums, in.Value); err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT id,attributes_json,created_at,updated_at FROM contacts WHERE json_extract(attributes_json,?)=? ORDER BY updated_at DESC LIMIT ?`, "$."+in.AttributeKey, in.Value, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var id, attrsRaw, createdAt, updatedAt string
		if err := rows.Scan(&id, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var attributes map[string]any
		_ = json.Unmarshal([]byte(attrsRaw), &attributes)
		result = append(result, map[string]any{"contact_id": id, "attributes": attributes, "created_at": createdAt, "updated_at": updatedAt})
	}
	return result, rows.Err()
}

func (s *Service) TrackEvent(ctx context.Context, in TrackEventInput) (map[string]any, error) {
	if in.ContactID == "" {
		return nil, errors.New("contact_id is required")
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	var defID string
	var version int
	var allowExtra bool
	if err := db.QueryRowContext(ctx, `SELECT id,version,allow_extra_parameters FROM event_definitions WHERE event_key=? AND status='active'`, in.EventKey).Scan(&defID, &version, &allowExtra); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("event %q is not defined", in.EventKey)
		}
		return nil, err
	}
	if err := validateEventProperties(ctx, db, defID, allowExtra, in.Properties); err != nil {
		return nil, err
	}
	occurred := in.OccurredAt
	if occurred == "" {
		occurred = nowUTC()
	} else if _, err := time.Parse(time.RFC3339, occurred); err != nil {
		return nil, errors.New("occurred_at must be RFC3339")
	}
	raw, _ := json.Marshal(in.Properties)
	eventID := newID("event_")
	now := nowUTC()
	_, err = db.ExecContext(ctx, `INSERT INTO events(id,contact_id,event_key,definition_version,occurred_at,properties_json,idempotency_key,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, eventID, in.ContactID, in.EventKey, version, occurred, string(raw), nullable(in.IdempotencyKey), now)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: events.idempotency_key") {
		if err := db.QueryRowContext(ctx, `SELECT id FROM events WHERE idempotency_key=?`, in.IdempotencyKey).Scan(&eventID); err != nil {
			return nil, err
		}
		return map[string]any{"database_id": in.DatabaseID, "event_id": eventID, "duplicate": true}, nil
	}
	if err != nil {
		return nil, err
	}
	_ = s.enrollTriggers(ctx, db, in.ContactID, in.EventKey, eventID)
	return map[string]any{"database_id": in.DatabaseID, "event_id": eventID, "duplicate": false}, nil
}

func normalizeIdentity(kind, value string) (string, error) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	value = strings.TrimSpace(value)
	if err := validateKey("identity type", kind); err != nil {
		return "", err
	}
	if value == "" {
		return "", errors.New("identity value is required")
	}
	switch kind {
	case "email":
		return strings.ToLower(value), nil
	case "phone":
		var b strings.Builder
		for i, r := range value {
			if r >= '0' && r <= '9' || (r == '+' && i == 0) {
				b.WriteRune(r)
			}
		}
		if b.Len() < 7 {
			return "", errors.New("phone identity is invalid")
		}
		return b.String(), nil
	default:
		return value, nil
	}
}

func validateAttributes(ctx context.Context, db *sql.DB, attrs map[string]any) error {
	defs, err := loadDefinitions(ctx, db, `SELECT attribute_key,data_type,is_required,enum_values_json FROM contact_attribute_definitions WHERE status='active'`)
	if err != nil {
		return err
	}
	for key, value := range attrs {
		def, ok := defs[key]
		if !ok {
			return fmt.Errorf("contact attribute %q is not defined", key)
		}
		if err := validateTypedValue(key, def.DataType, def.Enums, value); err != nil {
			return err
		}
	}
	return nil
}

type queryContexter interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func validateRequiredAttributes(ctx context.Context, queryer queryContexter, attrs map[string]any) error {
	rows, err := queryer.QueryContext(ctx, `SELECT attribute_key FROM contact_attribute_definitions WHERE status='active' AND is_required=1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return err
		}
		if _, ok := attrs[key]; !ok {
			return fmt.Errorf("required contact attribute %q is missing", key)
		}
	}
	return rows.Err()
}

type fieldDefinition struct {
	DataType string
	Required bool
	Enums    []string
}

func loadDefinitions(ctx context.Context, db *sql.DB, query string, args ...any) (map[string]fieldDefinition, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]fieldDefinition{}
	for rows.Next() {
		var key, typ string
		var required bool
		var raw sql.NullString
		if err := rows.Scan(&key, &typ, &required, &raw); err != nil {
			return nil, err
		}
		var enums []string
		if raw.Valid {
			_ = json.Unmarshal([]byte(raw.String), &enums)
		}
		result[key] = fieldDefinition{DataType: typ, Required: required, Enums: enums}
	}
	return result, rows.Err()
}

func validateEventProperties(ctx context.Context, db *sql.DB, defID string, allowExtra bool, properties map[string]any) error {
	defs, err := loadDefinitions(ctx, db, `SELECT parameter_key,data_type,is_required,enum_values_json FROM event_parameter_definitions WHERE event_definition_id=? AND status='active'`, defID)
	if err != nil {
		return err
	}
	for key, value := range properties {
		def, ok := defs[key]
		if !ok {
			if allowExtra {
				continue
			}
			return fmt.Errorf("event parameter %q is not defined", key)
		}
		if err := validateTypedValue(key, def.DataType, def.Enums, value); err != nil {
			return err
		}
	}
	for key, def := range defs {
		if def.Required {
			if _, ok := properties[key]; !ok {
				return fmt.Errorf("required event parameter %q is missing", key)
			}
		}
	}
	return nil
}

func validateTypedValue(key, typ string, enums []string, value any) error {
	valid := false
	switch typ {
	case "string":
		_, valid = value.(string)
	case "integer":
		switch v := value.(type) {
		case int, int32, int64, json.Number:
			valid = true
		case float64:
			valid = v == float64(int64(v))
		}
	case "number":
		switch value.(type) {
		case int, int32, int64, float32, float64, json.Number:
			valid = true
		}
	case "boolean":
		_, valid = value.(bool)
	case "object":
		_, valid = value.(map[string]any)
	case "array":
		_, valid = value.([]any)
		if !valid {
			_, valid = value.([]string)
		}
	case "date":
		if v, ok := value.(string); ok {
			_, err := time.Parse("2006-01-02", v)
			valid = err == nil
		}
	case "datetime":
		if v, ok := value.(string); ok {
			_, err := time.Parse(time.RFC3339, v)
			valid = err == nil
		}
	case "enum":
		if v, ok := value.(string); ok {
			valid = contains(enums, v)
		}
	}
	if !valid {
		return fmt.Errorf("field %q must be %s", key, typ)
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func queryMaps(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		item := map[string]any{}
		for i, col := range cols {
			if b, ok := values[i].([]byte); ok {
				item[col] = string(b)
			} else {
				item[col] = values[i]
			}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func namedArgs(values map[string]any) []any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		args = append(args, sql.Named(key, values[key]))
	}
	return args
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		f, e := strconv.ParseFloat(string(v), 64)
		return f, e == nil
	}
	return 0, false
}

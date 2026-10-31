package cdp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

func (s *Service) CreateSegment(ctx context.Context, in CreateSegmentInput) (map[string]any, error) {
	if in.Type != "rule" && in.Type != "sql" {
		return nil, errors.New("segment type must be rule or sql")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("segment name is required")
	}
	if in.Type == "rule" && len(in.Rule) == 0 {
		return nil, errors.New("rule is required for a rule segment")
	}
	if in.Type == "sql" {
		if err := validateSegmentSQL(in.SQL); err != nil {
			return nil, err
		}
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	ruleJSON, _ := json.Marshal(in.Rule)
	paramsJSON, _ := json.Marshal(in.Parameters)
	id := newID("segment_")
	now := nowUTC()
	_, err = db.ExecContext(ctx, `INSERT INTO segments
		(id,name,description,definition_type,definition_json,sql_query,sql_params_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, id, in.Name, nullable(in.Description), in.Type, nullableJSON(in.Rule, ruleJSON),
		nullable(in.SQL), nullableJSON(in.Parameters, paramsJSON), now, now)
	if err != nil {
		return nil, err
	}
	preview, err := s.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: in.DatabaseID, SegmentID: id})
	if err != nil {
		return map[string]any{"database_id": in.DatabaseID, "segment_id": id, "status": "draft", "validation_error": err.Error()}, nil
	}
	return map[string]any{"database_id": in.DatabaseID, "segment_id": id, "status": "validated", "count": preview.Count}, nil
}

func nullableJSON(value map[string]any, raw []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(raw)
}

func (s *Service) PreviewSegment(ctx context.Context, in PreviewSegmentInput) (*SegmentPreview, error) {
	started := time.Now()
	ids, err := s.evaluateSegment(ctx, in.DatabaseID, in.SegmentID, 100001)
	if err != nil {
		return nil, err
	}
	if len(ids) > 100000 {
		return nil, errors.New("segment exceeds the 100000-contact evaluation limit")
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err == nil {
		_, _ = db.ExecContext(ctx, `UPDATE segments SET status='validated',last_validated_at=?,updated_at=? WHERE id=?`, nowUTC(), nowUTC(), in.SegmentID)
	}
	sample := ids
	if len(sample) > 20 {
		sample = sample[:20]
	}
	return &SegmentPreview{DatabaseID: in.DatabaseID, SegmentID: in.SegmentID, Count: len(ids), SampleContactIDs: sample, DurationMS: time.Since(started).Milliseconds()}, nil
}

func (s *Service) PreviewSegmentsAcrossDatabases(ctx context.Context, in MultiDatabaseSegmentPreviewInput) map[string]any {
	results := make([]map[string]any, 0, len(in.Targets))
	total := 0
	partial := false
	seen := map[string]bool{}
	for _, target := range in.Targets {
		key := target.DatabaseID + "\x00" + target.SegmentID
		if seen[key] {
			continue
		}
		seen[key] = true
		preview, err := s.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: target.DatabaseID, SegmentID: target.SegmentID})
		if err != nil {
			partial = true
			results = append(results, map[string]any{"database_id": target.DatabaseID, "segment_id": target.SegmentID, "error": err.Error()})
			continue
		}
		total += preview.Count
		results = append(results, map[string]any{"database_id": target.DatabaseID, "segment_id": target.SegmentID, "count": preview.Count, "sample_contact_ids": preview.SampleContactIDs, "duration_ms": preview.DurationMS})
	}
	return map[string]any{"results": results, "total": total, "partial": partial}
}

func (s *Service) evaluateSegment(ctx context.Context, databaseID, segmentID string, limit int) ([]string, error) {
	db, err := s.manager.Get(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	var typ string
	var ruleRaw, queryRaw, paramsRaw sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT definition_type,definition_json,sql_query,sql_params_json FROM segments WHERE id=?`, segmentID).Scan(&typ, &ruleRaw, &queryRaw, &paramsRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("segment %q not found", segmentID)
		}
		return nil, err
	}
	query := ""
	var args []any
	queryDB := db
	var closeDB *sql.DB
	if typ == "rule" {
		var rule any
		if err := json.Unmarshal([]byte(ruleRaw.String), &rule); err != nil {
			return nil, errors.New("segment rule is invalid JSON")
		}
		where, values, err := compileRule(ctx, db, rule)
		if err != nil {
			return nil, err
		}
		query = `SELECT DISTINCT c.id AS contact_id FROM contacts c WHERE ` + where
		args = values
	} else {
		if err := validateSegmentSQL(queryRaw.String); err != nil {
			return nil, err
		}
		query = queryRaw.String
		var params map[string]any
		if paramsRaw.Valid {
			if err := json.Unmarshal([]byte(paramsRaw.String), &params); err != nil {
				return nil, err
			}
		}
		args = namedArgs(params)
		closeDB, err = s.manager.ReadOnly(ctx, databaseID)
		if err != nil {
			return nil, err
		}
		defer closeDB.Close()
		queryDB = closeDB
	}
	timed, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := queryDB.QueryContext(timed, query, args...)
	if err != nil {
		return nil, fmt.Errorf("segment query failed: %w", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(columns) != 1 || !strings.EqualFold(columns[0], "contact_id") {
		return nil, errors.New("segment query must return exactly one column named contact_id")
	}
	ids := make([]string, 0)
	seen := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if len(ids) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

var forbiddenSegmentSQL = regexp.MustCompile(`(?i)\b(attach|detach|pragma|insert|update|delete|drop|alter|create|replace|vacuum|reindex|load_extension)\b`)

func validateSegmentSQL(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return errors.New("sql is required for an sql segment")
	}
	withoutTrailing := strings.TrimSuffix(query, ";")
	if strings.Contains(withoutTrailing, ";") {
		return errors.New("segment SQL must contain exactly one statement")
	}
	upper := strings.ToUpper(strings.TrimSpace(withoutTrailing))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return errors.New("segment SQL must be a SELECT or WITH query")
	}
	if forbiddenSegmentSQL.MatchString(withoutTrailing) {
		return errors.New("segment SQL contains a prohibited operation")
	}
	return nil
}

func compileRule(ctx context.Context, db *sql.DB, node any) (string, []any, error) {
	object, ok := node.(map[string]any)
	if !ok {
		return "", nil, errors.New("each segment rule node must be an object")
	}
	for _, group := range []struct{ key, join string }{{"all", " AND "}, {"any", " OR "}} {
		if raw, exists := object[group.key]; exists {
			items, ok := raw.([]any)
			if !ok || len(items) == 0 {
				return "", nil, fmt.Errorf("%s must be a non-empty array", group.key)
			}
			parts := make([]string, 0, len(items))
			var args []any
			for _, item := range items {
				part, values, err := compileRule(ctx, db, item)
				if err != nil {
					return "", nil, err
				}
				parts = append(parts, "("+part+")")
				args = append(args, values...)
			}
			return strings.Join(parts, group.join), args, nil
		}
	}
	if event, ok := object["event"].(string); ok {
		if err := validateKey("event", event); err != nil {
			return "", nil, err
		}
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM event_definitions WHERE event_key=? AND status='active')`, event).Scan(&exists); err != nil {
			return "", nil, err
		}
		if exists == 0 {
			return "", nil, fmt.Errorf("event %q is not defined", event)
		}
		op, _ := object["operator"].(string)
		if op == "" {
			op = "occurred"
		}
		within, hasWithin := asFloat(object["within_days"])
		timeClause := ""
		args := []any{event}
		if hasWithin {
			if within <= 0 {
				return "", nil, errors.New("within_days must be positive")
			}
			timeClause = " AND e.occurred_at >= datetime('now', ?)"
			args = append(args, fmt.Sprintf("-%g days", within))
		}
		switch op {
		case "occurred":
			return `EXISTS(SELECT 1 FROM events e WHERE e.contact_id=c.id AND e.event_key=?` + timeClause + `)`, args, nil
		case "not_occurred":
			return `NOT EXISTS(SELECT 1 FROM events e WHERE e.contact_id=c.id AND e.event_key=?` + timeClause + `)`, args, nil
		case "count_at_least":
			count, ok := asFloat(object["value"])
			if !ok {
				return "", nil, errors.New("count_at_least requires a numeric value")
			}
			args = append(args, count)
			return `(SELECT COUNT(*) FROM events e WHERE e.contact_id=c.id AND e.event_key=?` + timeClause + `) >= ?`, args, nil
		default:
			return "", nil, fmt.Errorf("unsupported event operator %q", op)
		}
	}
	field, ok := object["field"].(string)
	if !ok {
		return "", nil, errors.New("rule must contain all, any, event, or field")
	}
	field = strings.TrimPrefix(field, "contact.")
	if err := validateKey("field", field); err != nil {
		return "", nil, err
	}
	var typ string
	var filterable bool
	var enumRaw sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT data_type,is_filterable,enum_values_json FROM contact_attribute_definitions WHERE attribute_key=? AND status='active'`, field).Scan(&typ, &filterable, &enumRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, fmt.Errorf("contact field %q is not defined", field)
		}
		return "", nil, err
	}
	if !filterable {
		return "", nil, fmt.Errorf("contact field %q is not filterable", field)
	}
	op, _ := object["operator"].(string)
	value := object["value"]
	path := "$." + field
	if !contains([]string{"exists", "not_exists"}, op) {
		var enumValues []string
		if enumRaw.Valid {
			_ = json.Unmarshal([]byte(enumRaw.String), &enumValues)
		}
		if err := validateTypedValue(field, typ, enumValues, value); err != nil {
			return "", nil, err
		}
	}
	switch op {
	case "equals":
		return `json_extract(c.attributes_json, ?) = ?`, []any{path, value}, nil
	case "not_equals":
		return `json_extract(c.attributes_json, ?) != ?`, []any{path, value}, nil
	case "greater_than":
		return `json_extract(c.attributes_json, ?) > ?`, []any{path, value}, nil
	case "less_than":
		return `json_extract(c.attributes_json, ?) < ?`, []any{path, value}, nil
	case "contains":
		return `CAST(json_extract(c.attributes_json, ?) AS TEXT) LIKE ?`, []any{path, "%" + fmt.Sprint(value) + "%"}, nil
	case "exists":
		return `json_type(c.attributes_json, ?) IS NOT NULL`, []any{path}, nil
	case "not_exists":
		return `json_type(c.attributes_json, ?) IS NULL`, []any{path}, nil
	default:
		return "", nil, fmt.Errorf("unsupported field operator %q for %s", op, typ)
	}
}

func (s *Service) materializeSegment(ctx context.Context, databaseID, segmentID string) ([]string, int, error) {
	ids, err := s.evaluateSegment(ctx, databaseID, segmentID, 100001)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) > 100000 {
		return nil, 0, errors.New("segment exceeds materialization limit")
	}
	db, err := s.manager.Get(ctx, databaseID)
	if err != nil {
		return nil, 0, err
	}
	var version int
	if err := db.QueryRowContext(ctx, `SELECT version FROM segments WHERE id=?`, segmentID).Scan(&version); err != nil {
		return nil, 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM segment_memberships WHERE segment_id=? AND segment_version=?`, segmentID, version); err != nil {
		return nil, 0, err
	}
	now := nowUTC()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO segment_memberships(segment_id,segment_version,contact_id,evaluated_at) VALUES(?,?,?,?)`, segmentID, version, id, now); err != nil {
			return nil, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return ids, version, nil
}

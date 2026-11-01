package cdp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const defaultPageSize = 20
const maxPageSize = 100

func normalizePage(page, pageSize int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize, (page - 1) * pageSize
}

func pageResult(rows []map[string]any, page, pageSize, total int) *PageResult {
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return &PageResult{Rows: rows, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}
}

func countQuery(ctx context.Context, db *sql.DB, query string, args ...any) (int, error) {
	var total int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Service) ListContacts(ctx context.Context, in PageInput) (*PageResult, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	page, pageSize, offset := normalizePage(in.Page, in.PageSize)
	total, err := countQuery(ctx, db, `SELECT COUNT(*) FROM contacts`)
	if err != nil {
		return nil, err
	}
	rows, err := queryMaps(ctx, db, `SELECT c.id AS contact_id,
		(SELECT ci.original_value FROM contact_identities ci WHERE ci.contact_id=c.id
		 ORDER BY ci.is_primary DESC,ci.created_at LIMIT 1) AS primary_identity,
		c.attributes_json,c.created_at,c.updated_at
		FROM contacts c ORDER BY c.updated_at DESC,c.id LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		raw, _ := row["attributes_json"].(string)
		attributes := map[string]any{}
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &attributes)
		}
		row["attributes"] = attributes
		delete(row, "attributes_json")
	}
	return pageResult(rows, page, pageSize, total), nil
}

func (s *Service) ListSegments(ctx context.Context, in StatusPageInput) (*PageResult, error) {
	return s.listStatusPage(ctx, in, "segments",
		`SELECT id AS segment_id,name,description,definition_type,status,version,last_validated_at,created_at,updated_at
		 FROM segments`, "updated_at DESC,id")
}

func (s *Service) ListMessageTemplates(ctx context.Context, in StatusPageInput) (*PageResult, error) {
	return s.listStatusPage(ctx, in, "message_templates",
		`SELECT id AS template_id,name,channel,subject,status,created_at,updated_at FROM message_templates`,
		"updated_at DESC,id")
}

func (s *Service) ListCampaigns(ctx context.Context, in StatusPageInput) (*PageResult, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	page, pageSize, offset := normalizePage(in.Page, in.PageSize)
	where, args := statusWhereWithPrefix(in.Status, "c.")
	total, err := countQuery(ctx, db, `SELECT COUNT(*) FROM campaigns c`+where, args...)
	if err != nil {
		return nil, err
	}
	queryArgs := append(args, pageSize, offset)
	rows, err := queryMaps(ctx, db, `SELECT c.id AS campaign_id,c.name,c.status,
		c.segment_id,s.name AS segment_name,c.template_id,t.name AS template_name,t.channel,
		(SELECT COUNT(*) FROM campaign_audience ca WHERE ca.campaign_id=c.id) AS audience_count,
		c.created_at,c.launched_at
		FROM campaigns c JOIN segments s ON s.id=c.segment_id JOIN message_templates t ON t.id=c.template_id`+
		where+` ORDER BY c.created_at DESC,c.id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	return pageResult(rows, page, pageSize, total), nil
}

func (s *Service) ListJourneys(ctx context.Context, in StatusPageInput) (*PageResult, error) {
	return s.listStatusPage(ctx, in, "journeys",
		`SELECT id AS journey_id,name,description,status,timezone,created_at,updated_at FROM journeys`,
		"updated_at DESC,id")
}

func (s *Service) listStatusPage(ctx context.Context, in StatusPageInput, table, selectSQL, orderBy string) (*PageResult, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	page, pageSize, offset := normalizePage(in.Page, in.PageSize)
	where, args := statusWhere(in.Status)
	total, err := countQuery(ctx, db, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)+where, args...)
	if err != nil {
		return nil, err
	}
	queryArgs := append(args, pageSize, offset)
	rows, err := queryMaps(ctx, db, selectSQL+where+" ORDER BY "+orderBy+" LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return nil, err
	}
	return pageResult(rows, page, pageSize, total), nil
}

func statusWhere(status string) (string, []any) {
	return statusWhereWithPrefix(status, "")
}

func statusWhereWithPrefix(status, prefix string) (string, []any) {
	if status == "" {
		return "", nil
	}
	return " WHERE " + prefix + "status=?", []any{status}
}

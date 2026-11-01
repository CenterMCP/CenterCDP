package cdp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Service) SaveTemplate(ctx context.Context, in SaveTemplateInput) (map[string]any, error) {
	if in.Channel != "email" && in.Channel != "sms" && in.Channel != "push" {
		return nil, errors.New("channel must be email, sms, or push")
	}
	if strings.TrimSpace(in.Body) == "" {
		return nil, errors.New("template body is required")
	}
	if in.Channel == "email" && strings.TrimSpace(in.Subject) == "" {
		return nil, errors.New("email subject is required")
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	id := newID("template_")
	now := nowUTC()
	status := "draft"
	if in.Active {
		status = "active"
	}
	_, err = db.ExecContext(ctx, `INSERT INTO message_templates(id,name,channel,subject,body,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, id, in.Name, in.Channel, nullable(in.Subject), in.Body, status, now, now)
	if err != nil {
		return nil, err
	}
	return map[string]any{"database_id": in.DatabaseID, "template_id": id, "status": status}, nil
}

func (s *Service) CreateCampaign(ctx context.Context, in CreateCampaignInput) (map[string]any, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	var segmentOK, templateOK int
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM segments WHERE id=? AND status='validated')`, in.SegmentID).Scan(&segmentOK); err != nil {
		return nil, err
	}
	if segmentOK == 0 {
		return nil, errors.New("campaign segment must exist and be validated")
	}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM message_templates WHERE id=? AND status='active')`, in.TemplateID).Scan(&templateOK); err != nil {
		return nil, err
	}
	if templateOK == 0 {
		return nil, errors.New("campaign template must exist and be active")
	}
	id := newID("campaign_")
	now := nowUTC()
	_, err = db.ExecContext(ctx, `INSERT INTO campaigns(id,name,segment_id,template_id,status,created_at) VALUES(?,?,?,?, 'ready',?)`, id, in.Name, in.SegmentID, in.TemplateID, now)
	if err != nil {
		return nil, err
	}
	return map[string]any{"database_id": in.DatabaseID, "campaign_id": id, "status": "ready"}, nil
}

func (s *Service) LaunchCampaign(ctx context.Context, in CampaignInput) (map[string]any, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	var segmentID, templateID, status, channel string
	if err := db.QueryRowContext(ctx, `SELECT c.segment_id,c.template_id,c.status,t.channel FROM campaigns c JOIN message_templates t ON t.id=c.template_id WHERE c.id=?`, in.CampaignID).Scan(&segmentID, &templateID, &status, &channel); err != nil {
		return nil, err
	}
	if status != "ready" && status != "paused" {
		return nil, fmt.Errorf("campaign must be ready or paused, current status is %s", status)
	}
	ids, _, err := s.materializeSegment(ctx, in.DatabaseID, segmentID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := nowUTC()
	queued, suppressed := 0, 0
	for _, contactID := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO campaign_audience(campaign_id,contact_id,created_at) VALUES(?,?,?)`, in.CampaignID, contactID, now); err != nil {
			return nil, err
		}
		recipient, allowed, err := campaignRecipient(ctx, tx, contactID, channel)
		if err != nil {
			return nil, err
		}
		if !allowed {
			_, _ = tx.ExecContext(ctx, `UPDATE campaign_audience SET status='suppressed' WHERE campaign_id=? AND contact_id=?`, in.CampaignID, contactID)
			suppressed++
			continue
		}
		idempotency := "campaign:" + in.CampaignID + ":contact:" + contactID
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO message_outbox
			(id,contact_id,campaign_id,channel,template_id,recipient,idempotency_key,status,scheduled_at,created_at)
			VALUES(?,?,?,?,?,?,?,'pending',?,?)`, newID("message_"), contactID, in.CampaignID, channel, templateID, recipient, idempotency, now, now)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			queued++
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaigns SET status='running',launched_at=COALESCE(launched_at,?) WHERE id=?`, now, in.CampaignID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"database_id": in.DatabaseID, "campaign_id": in.CampaignID, "audience": len(ids), "queued": queued, "suppressed": suppressed, "status": "running"}, nil
}

func campaignRecipient(ctx context.Context, tx *sql.Tx, contactID, channel string) (string, bool, error) {
	identityType := channel
	if channel == "push" {
		identityType = "push_token"
	}
	var recipient string
	err := tx.QueryRowContext(ctx, `SELECT original_value FROM contact_identities WHERE contact_id=? AND identity_type=? ORDER BY is_primary DESC,created_at LIMIT 1`, contactID, identityType).Scan(&recipient)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	consentKey := channel + "_marketing"
	var granted bool
	err = tx.QueryRowContext(ctx, `SELECT granted FROM contact_consents WHERE contact_id=? AND consent_key=?`, contactID, consentKey).Scan(&granted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return recipient, granted, nil
}

func (s *Service) ListOutbox(ctx context.Context, in OutboxListInput) (*PageResult, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	pageSize := in.PageSize
	if pageSize == 0 && in.Limit > 0 {
		pageSize = in.Limit
	}
	page, pageSize, offset := normalizePage(in.Page, pageSize)
	where, args := statusWhere(in.Status)
	total, err := countQuery(ctx, db, `SELECT COUNT(*) FROM message_outbox`+where, args...)
	if err != nil {
		return nil, err
	}
	queryArgs := append(args, pageSize, offset)
	rows, err := queryMaps(ctx, db, `SELECT id,contact_id,campaign_id,channel,template_id,recipient,status,scheduled_at,attempt,last_error
		FROM message_outbox`+where+` ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	return pageResult(rows, page, pageSize, total), nil
}

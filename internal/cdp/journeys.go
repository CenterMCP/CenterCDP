package cdp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Service) CreateJourney(ctx context.Context, in CreateJourneyInput) (map[string]any, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("journey name is required")
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return nil, fmt.Errorf("invalid journey timezone %q", in.Timezone)
	}
	if err := validateJourneyGraph(in.Steps, in.Transitions); err != nil {
		return nil, err
	}
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	if err := s.validateJourneyReferences(ctx, db, in.Steps); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	journeyID := newID("journey_")
	versionID := newID("journeyv_")
	now := nowUTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO journeys(id,name,description,status,timezone,created_at,updated_at) VALUES(?,?,?,'draft',?,?,?)`, journeyID, in.Name, nullable(in.Description), in.Timezone, now, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journey_versions(id,journey_id,version,status,created_at) VALUES(?,?,1,'draft',?)`, versionID, journeyID, now); err != nil {
		return nil, err
	}
	stepIDs := map[string]string{}
	for _, step := range in.Steps {
		id := newID("step_")
		stepIDs[step.Key] = id
		raw, _ := json.Marshal(step.Configuration)
		if _, err := tx.ExecContext(ctx, `INSERT INTO journey_steps(id,journey_version_id,step_key,step_type,name,configuration_json) VALUES(?,?,?,?,?,?)`, id, versionID, step.Key, step.Type, step.Name, string(raw)); err != nil {
			return nil, err
		}
	}
	for _, edge := range in.Transitions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO journey_transitions(id,journey_version_id,from_step_id,outcome,to_step_id,priority) VALUES(?,?,?,?,?,?)`, newID("edge_"), versionID, stepIDs[edge.From], edge.Outcome, stepIDs[edge.To], edge.Priority); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"database_id": in.DatabaseID, "journey_id": journeyID, "version": 1, "status": "draft"}, nil
}

func validateJourneyGraph(steps []JourneyStepInput, transitions []JourneyTransitionInput) error {
	if len(steps) < 2 {
		return errors.New("a journey needs at least a trigger and an end step")
	}
	known := map[string]string{}
	triggerKey := ""
	triggers, ends := 0, 0
	for _, step := range steps {
		if err := validateKey("step key", step.Key); err != nil {
			return err
		}
		if _, exists := known[step.Key]; exists {
			return fmt.Errorf("duplicate step key %q", step.Key)
		}
		if !contains([]string{"trigger", "action", "delay", "condition", "end"}, step.Type) {
			return fmt.Errorf("unsupported step type %q", step.Type)
		}
		known[step.Key] = step.Type
		if step.Type == "trigger" {
			triggers++
			triggerKey = step.Key
		}
		if step.Type == "end" {
			ends++
		}
	}
	if triggers != 1 {
		return errors.New("a journey must have exactly one trigger step")
	}
	if ends == 0 {
		return errors.New("a journey must have at least one end step")
	}
	outcomes := map[string]bool{}
	for _, edge := range transitions {
		if known[edge.From] == "" || known[edge.To] == "" {
			return fmt.Errorf("transition %s -> %s references an unknown step", edge.From, edge.To)
		}
		if known[edge.From] == "end" {
			return fmt.Errorf("end step %q cannot have outgoing transitions", edge.From)
		}
		key := edge.From + "\x00" + edge.Outcome
		if outcomes[key] {
			return fmt.Errorf("duplicate outcome %q from step %q", edge.Outcome, edge.From)
		}
		outcomes[key] = true
	}
	for key, typ := range known {
		if typ == "end" {
			continue
		}
		if typ == "condition" {
			if !outcomes[key+"\x00matched"] || !outcomes[key+"\x00not_matched"] {
				return fmt.Errorf("condition %q needs matched and not_matched transitions", key)
			}
		} else if !outcomes[key+"\x00default"] {
			return fmt.Errorf("step %q needs a default transition", key)
		}
	}
	adjacency := map[string][]string{}
	for _, edge := range transitions {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}
	colors := map[string]int{}
	var visit func(string) error
	visit = func(key string) error {
		if colors[key] == 1 {
			return fmt.Errorf("journey contains a cycle at step %q", key)
		}
		if colors[key] == 2 {
			return nil
		}
		colors[key] = 1
		for _, next := range adjacency[key] {
			if err := visit(next); err != nil {
				return err
			}
		}
		colors[key] = 2
		return nil
	}
	if err := visit(triggerKey); err != nil {
		return err
	}
	for key := range known {
		if colors[key] == 0 {
			return fmt.Errorf("journey step %q is unreachable from the trigger", key)
		}
	}
	return nil
}

func (s *Service) validateJourneyReferences(ctx context.Context, db *sql.DB, steps []JourneyStepInput) error {
	for _, step := range steps {
		switch step.Type {
		case "trigger":
			typ, _ := step.Configuration["type"].(string)
			if typ != "contact_created" && typ != "event" {
				return fmt.Errorf("trigger %q type must be contact_created or event", step.Key)
			}
			if typ == "event" {
				if err := requireDefinedEvent(ctx, db, fmt.Sprint(step.Configuration["event_key"])); err != nil {
					return err
				}
			}
		case "action":
			if fmt.Sprint(step.Configuration["type"]) != "send_message" {
				return fmt.Errorf("action %q currently supports only send_message", step.Key)
			}
			templateID := fmt.Sprint(step.Configuration["template_id"])
			var exists int
			if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM message_templates WHERE id=? AND status='active')`, templateID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				return fmt.Errorf("action %q references an inactive or missing template", step.Key)
			}
		case "delay":
			amount, ok := asFloat(step.Configuration["duration"])
			if !ok || amount <= 0 {
				return fmt.Errorf("delay %q needs a positive duration", step.Key)
			}
			unit := fmt.Sprint(step.Configuration["unit"])
			if !contains([]string{"minutes", "hours", "days"}, unit) {
				return fmt.Errorf("delay %q unit must be minutes, hours, or days", step.Key)
			}
		case "condition":
			if fmt.Sprint(step.Configuration["type"]) != "event_occurred" {
				return fmt.Errorf("condition %q currently supports only event_occurred", step.Key)
			}
			if err := requireDefinedEvent(ctx, db, fmt.Sprint(step.Configuration["event_key"])); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireDefinedEvent(ctx context.Context, db *sql.DB, key string) error {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM event_definitions WHERE event_key=? AND status='active')`, key).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("event %q is not defined", key)
	}
	return nil
}

func (s *Service) PublishJourney(ctx context.Context, in JourneyInput) (map[string]any, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var versionID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM journey_versions WHERE journey_id=? AND status='draft' ORDER BY version DESC LIMIT 1`, in.JourneyID).Scan(&versionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("journey has no draft version")
		}
		return nil, err
	}
	now := nowUTC()
	if _, err := tx.ExecContext(ctx, `UPDATE journey_versions SET status='retired' WHERE journey_id=? AND status='published'`, in.JourneyID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE journey_versions SET status='published',published_at=? WHERE id=?`, now, versionID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE journeys SET status='active',updated_at=? WHERE id=?`, now, in.JourneyID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"database_id": in.DatabaseID, "journey_id": in.JourneyID, "status": "active"}, nil
}

func (s *Service) PauseJourney(ctx context.Context, in JourneyInput) (map[string]any, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	res, err := db.ExecContext(ctx, `UPDATE journeys SET status='paused',updated_at=? WHERE id=? AND status='active'`, nowUTC(), in.JourneyID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, errors.New("active journey not found")
	}
	return map[string]any{"database_id": in.DatabaseID, "journey_id": in.JourneyID, "status": "paused"}, nil
}

func (s *Service) enrollTriggers(ctx context.Context, db *sql.DB, contactID, triggerKey, eventID string) error {
	rows, err := db.QueryContext(ctx, `SELECT v.id,s.id,s.configuration_json
		FROM journeys j JOIN journey_versions v ON v.journey_id=j.id AND v.status='published'
		JOIN journey_steps s ON s.journey_version_id=v.id AND s.step_type='trigger'
		WHERE j.status='active'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type match struct {
		versionID, triggerID string
		config               map[string]any
	}
	var matches []match
	for rows.Next() {
		var item match
		var raw string
		if err := rows.Scan(&item.versionID, &item.triggerID, &raw); err != nil {
			return err
		}
		_ = json.Unmarshal([]byte(raw), &item.config)
		matches = append(matches, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range matches {
		typ := fmt.Sprint(item.config["type"])
		matched := typ == "contact_created" && triggerKey == "contact_created"
		if typ == "event" && fmt.Sprint(item.config["event_key"]) == triggerKey {
			matched = true
		}
		if !matched {
			continue
		}
		var nextStep string
		if err := db.QueryRowContext(ctx, `SELECT to_step_id FROM journey_transitions WHERE from_step_id=? AND outcome='default'`, item.triggerID).Scan(&nextStep); err != nil {
			return err
		}
		enrollmentKey := item.versionID + ":" + contactID + ":" + eventID
		if eventID == "" {
			enrollmentKey = item.versionID + ":" + contactID + ":contact_created"
		}
		_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO journey_enrollments
			(id,journey_version_id,contact_id,trigger_event_id,enrollment_key,current_step_id,status,enrolled_at,next_run_at)
			VALUES(?,?,?,?,?,?,'active',?,?)`, newID("enrollment_"), item.versionID, contactID, nullable(eventID), enrollmentKey, nextStep, nowUTC(), nowUTC())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RunJourneyWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, err := s.manager.IDs(ctx)
			if err != nil {
				s.logger.Errorf("list journey databases: %v", err)
				continue
			}
			for _, id := range ids {
				if err := s.processDueEnrollments(ctx, id); err != nil {
					s.logger.Errorf("process journeys for %s: %v", id, err)
				}
			}
		}
	}
}

func (s *Service) processDueEnrollments(ctx context.Context, databaseID string) error {
	db, err := s.manager.Get(ctx, databaseID)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM journey_enrollments WHERE status IN ('active','waiting') AND next_run_at<=? ORDER BY next_run_at LIMIT 50`, nowUTC())
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.processEnrollment(ctx, db, id); err != nil {
			_, _ = db.ExecContext(ctx, `UPDATE journey_enrollments SET status='failed' WHERE id=?`, id)
			s.logger.Errorf("journey enrollment %s failed: %v", id, err)
		}
	}
	return nil
}

func (s *Service) processEnrollment(ctx context.Context, db *sql.DB, enrollmentID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var stepID, stepType, configRaw, contactID, enrolledAt string
	err = tx.QueryRowContext(ctx, `SELECT s.id,s.step_type,s.configuration_json,e.contact_id,e.enrolled_at
		FROM journey_enrollments e JOIN journey_steps s ON s.id=e.current_step_id
		WHERE e.id=? AND e.status IN ('active','waiting')`, enrollmentID).Scan(&stepID, &stepType, &configRaw, &contactID, &enrolledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var config map[string]any
	_ = json.Unmarshal([]byte(configRaw), &config)
	outcome := "default"
	result := map[string]any{}
	switch stepType {
	case "end":
		_, err = tx.ExecContext(ctx, `UPDATE journey_enrollments SET status='completed',completed_at=?,next_run_at=NULL WHERE id=?`, nowUTC(), enrollmentID)
		if err != nil {
			return err
		}
		return tx.Commit()
	case "delay":
		amount, _ := asFloat(config["duration"])
		duration := time.Duration(amount * float64(time.Minute))
		switch fmt.Sprint(config["unit"]) {
		case "hours":
			duration = time.Duration(amount * float64(time.Hour))
		case "days":
			duration = time.Duration(amount * float64(24*time.Hour))
		}
		next, err := nextJourneyStep(ctx, tx, stepID, "default")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE journey_enrollments SET current_step_id=?,status='waiting',next_run_at=? WHERE id=?`, next, time.Now().UTC().Add(duration).Format(time.RFC3339Nano), enrollmentID)
		if err != nil {
			return err
		}
		return tx.Commit()
	case "condition":
		var matched bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE contact_id=? AND event_key=? AND occurred_at>=?)`, contactID, fmt.Sprint(config["event_key"]), enrolledAt).Scan(&matched); err != nil {
			return err
		}
		if matched {
			outcome = "matched"
		} else {
			outcome = "not_matched"
		}
		result["matched"] = matched
	case "action":
		if fmt.Sprint(config["type"]) != "send_message" {
			return errors.New("unsupported journey action")
		}
		if err := s.enqueueJourneyMessage(ctx, tx, enrollmentID, stepID, contactID, config); err != nil {
			return err
		}
		result["queued"] = true
	default:
		return fmt.Errorf("cannot execute step type %q", stepType)
	}
	rawResult, _ := json.Marshal(result)
	runID := newID("steprun_")
	now := nowUTC()
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO journey_step_runs
		(id,enrollment_id,step_id,status,idempotency_key,scheduled_at,started_at,completed_at,result_json)
		VALUES(?,?,?,'completed',?,?,?,?,?)`, runID, enrollmentID, stepID, enrollmentID+":"+stepID, now, now, now, string(rawResult))
	if err != nil {
		return err
	}
	next, err := nextJourneyStep(ctx, tx, stepID, outcome)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE journey_enrollments SET current_step_id=?,status='active',next_run_at=? WHERE id=?`, next, nowUTC(), enrollmentID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func nextJourneyStep(ctx context.Context, tx *sql.Tx, stepID, outcome string) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT to_step_id FROM journey_transitions WHERE from_step_id=? AND outcome=? ORDER BY priority LIMIT 1`, stepID, outcome).Scan(&id); err != nil {
		return "", fmt.Errorf("journey transition %s/%s: %w", stepID, outcome, err)
	}
	return id, nil
}

func (s *Service) enqueueJourneyMessage(ctx context.Context, tx *sql.Tx, enrollmentID, stepID, contactID string, config map[string]any) error {
	templateID := fmt.Sprint(config["template_id"])
	var channel string
	if err := tx.QueryRowContext(ctx, `SELECT channel FROM message_templates WHERE id=? AND status='active'`, templateID).Scan(&channel); err != nil {
		return err
	}
	identityType := channel
	if channel == "push" {
		identityType = "push_token"
	}
	var recipient string
	if err := tx.QueryRowContext(ctx, `SELECT original_value FROM contact_identities WHERE contact_id=? AND identity_type=? ORDER BY is_primary DESC,created_at LIMIT 1`, contactID, identityType).Scan(&recipient); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	messageType := fmt.Sprint(config["message_type"])
	if messageType == "" {
		messageType = "marketing"
	}
	status := "pending"
	if messageType == "marketing" {
		var granted bool
		err := tx.QueryRowContext(ctx, `SELECT granted FROM contact_consents WHERE contact_id=? AND consent_key=?`, contactID, channel+"_marketing").Scan(&granted)
		if errors.Is(err, sql.ErrNoRows) || !granted {
			status = "suppressed"
		} else if err != nil {
			return err
		}
	}
	now := nowUTC()
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO message_outbox
		(id,contact_id,channel,template_id,recipient,idempotency_key,status,scheduled_at,created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, newID("message_"), contactID, channel, templateID, recipient, "journey:"+enrollmentID+":"+stepID, status, now, now)
	return err
}

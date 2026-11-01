package cdp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Demo records live in a reserved namespace so they can be removed without
// touching customer-created data. Keep every ID and dictionary key created by
// GenerateDemoData under this prefix.
const demoPrefix = "demo_"
const demoContactCount = 300

type demoContact struct {
	id         string
	name       string
	email      string
	phone      string
	country    string
	tier       string
	lifetime   float64
	daysAgo    int
	emailOptIn bool
	smsOptIn   bool
}

type demoEvent struct {
	id, contactID, eventKey string
	daysAgo                 int
	properties              map[string]any
}

// GenerateDemoData replaces the demo namespace with a deterministic sample
// dataset. Replacing instead of appending makes the tool safe to run repeatedly.
func (s *Service) GenerateDemoData(ctx context.Context, databaseID string) (map[string]any, error) {
	db, err := s.manager.Get(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	replaced, err := resetDemoDataTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("clear existing demo data: %w", err)
	}

	created := make(map[string]int64)
	exec := func(table, statement string, args ...any) error {
		result, err := tx.ExecContext(ctx, statement, args...)
		if err != nil {
			return fmt.Errorf("insert demo %s: %w", table, err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		created[table] += rows
		return nil
	}

	seededAt := time.Now().UTC()
	now := seededAt.Format(time.RFC3339Nano)
	attributeDefinitions := []struct {
		id, key, name, description, dataType string
		filterable, searchable, pii          bool
		enums                                []string
	}{
		{"demo_attr_first_name", "demo_first_name", "First name", "Demo contact first name", "string", true, true, true, nil},
		{"demo_attr_country", "demo_country", "Country", "Demo ISO country code", "enum", true, false, false, []string{"TH", "SG", "US"}},
		{"demo_attr_tier", "demo_tier", "Loyalty tier", "Demo loyalty tier", "enum", true, false, false, []string{"bronze", "silver", "gold"}},
		{"demo_attr_lifetime_value", "demo_lifetime_value", "Lifetime value", "Demo lifetime value in THB", "number", true, false, false, nil},
		{"demo_attr_last_seen_at", "demo_last_seen_at", "Last seen", "Most recent demo activity time", "datetime", true, false, false, nil},
	}
	for _, definition := range attributeDefinitions {
		enums, _ := json.Marshal(definition.enums)
		if err := exec("contact_attribute_definitions", `INSERT INTO contact_attribute_definitions
			(id,attribute_key,display_name,description,data_type,is_filterable,is_searchable,is_pii,enum_values_json,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`, definition.id, definition.key, definition.name, definition.description,
			definition.dataType, definition.filterable, definition.searchable, definition.pii, string(enums), now, now); err != nil {
			return nil, err
		}
	}

	type demoParameter struct {
		id, key, name, description, dataType string
		required, filterable, pii            bool
		enums                                []string
	}
	eventDefinitions := []struct {
		id, key, name, description string
		parameters                 []demoParameter
	}{
		{"demo_evtdef_page_view", "demo_page_view", "Page view", "Demo website page view", []demoParameter{
			{"demo_param_page_path", "page_path", "Page path", "Viewed page path", "string", true, true, false, nil},
			{"demo_param_device", "device", "Device", "Visitor device category", "enum", false, true, false, []string{"mobile", "desktop", "tablet"}},
		}},
		{"demo_evtdef_purchase", "demo_purchase", "Purchase", "Demo completed purchase", []demoParameter{
			{"demo_param_order_id", "order_id", "Order ID", "Demo commerce order ID", "string", true, true, false, nil},
			{"demo_param_amount", "amount", "Amount", "Order total in the selected currency", "number", true, true, false, nil},
			{"demo_param_currency", "currency", "Currency", "ISO transaction currency", "enum", true, true, false, []string{"THB", "SGD", "USD"}},
		}},
		{"demo_evtdef_cart_abandoned", "demo_cart_abandoned", "Cart abandoned", "Demo cart abandonment", []demoParameter{
			{"demo_param_cart_amount", "amount", "Cart amount", "Abandoned cart total", "number", true, true, false, nil},
			{"demo_param_item_count", "item_count", "Item count", "Number of items left in the cart", "integer", true, true, false, nil},
		}},
	}
	for _, definition := range eventDefinitions {
		if err := exec("event_definitions", `INSERT INTO event_definitions
			(id,event_key,display_name,description,allow_extra_parameters,created_at,updated_at)
			VALUES(?,?,?,?,0,?,?)`, definition.id, definition.key, definition.name, definition.description, now, now); err != nil {
			return nil, err
		}
		for _, parameter := range definition.parameters {
			enums, _ := json.Marshal(parameter.enums)
			if err := exec("event_parameter_definitions", `INSERT INTO event_parameter_definitions
				(id,event_definition_id,parameter_key,display_name,description,data_type,is_required,is_filterable,is_pii,enum_values_json)
				VALUES(?,?,?,?,?,?,?,?,?,?)`, parameter.id, definition.id, parameter.key, parameter.name, parameter.description,
				parameter.dataType, parameter.required, parameter.filterable, parameter.pii, string(enums)); err != nil {
				return nil, err
			}
		}
	}

	contacts := []demoContact{
		{"demo_contact_001", "Pim", "demo.pim@example.invalid", "+66810000001", "TH", "gold", 125000, 1, true, true},
		{"demo_contact_002", "Anan", "demo.anan@example.invalid", "+66810000002", "TH", "silver", 42000, 3, true, false},
		{"demo_contact_003", "Mei", "demo.mei@example.invalid", "+6580000003", "SG", "gold", 98000, 2, false, false},
		{"demo_contact_004", "James", "demo.james@example.invalid", "+12025550104", "US", "bronze", 12500, 14, true, false},
		{"demo_contact_005", "Nida", "demo.nida@example.invalid", "+66810000005", "TH", "bronze", 8500, 7, true, true},
		{"demo_contact_006", "Arun", "demo.arun@example.invalid", "+66810000006", "TH", "gold", 210000, 0, true, true},
	}
	countries := []string{"TH", "TH", "SG", "US"}
	tiers := []string{"bronze", "silver", "gold"}
	for number := len(contacts) + 1; number <= demoContactCount; number++ {
		contacts = append(contacts, demoContact{
			id:         fmt.Sprintf("demo_contact_%03d", number),
			name:       fmt.Sprintf("Customer %03d", number),
			email:      fmt.Sprintf("demo.customer%03d@example.invalid", number),
			phone:      fmt.Sprintf("+6682%07d", number),
			country:    countries[(number-1)%len(countries)],
			tier:       tiers[(number-1)%len(tiers)],
			lifetime:   float64(5000 + (number%20)*7500),
			daysAgo:    (number * 7) % 91,
			emailOptIn: number%5 != 0,
			smsOptIn:   number%3 == 0,
		})
	}
	for index, contact := range contacts {
		lastSeen := seededAt.AddDate(0, 0, -contact.daysAgo).Format(time.RFC3339)
		attributes, _ := json.Marshal(map[string]any{
			"demo_first_name":     contact.name,
			"demo_country":        contact.country,
			"demo_tier":           contact.tier,
			"demo_lifetime_value": contact.lifetime,
			"demo_last_seen_at":   lastSeen,
		})
		createdAt := seededAt.AddDate(0, -index-1, 0).Format(time.RFC3339Nano)
		if err := exec("contacts", `INSERT INTO contacts(id,attributes_json,created_at,updated_at) VALUES(?,?,?,?)`, contact.id, string(attributes), createdAt, now); err != nil {
			return nil, err
		}
		if err := exec("contact_identities", `INSERT INTO contact_identities
			(id,contact_id,identity_type,normalized_value,original_value,is_primary,created_at) VALUES(?,?,?,?,?,1,?)`,
			"demo_identity_email_"+contact.id[len(demoPrefix):], contact.id, "email", contact.email, contact.email, createdAt); err != nil {
			return nil, err
		}
		if err := exec("contact_identities", `INSERT INTO contact_identities
			(id,contact_id,identity_type,normalized_value,original_value,is_primary,created_at) VALUES(?,?,?,?,?,0,?)`,
			"demo_identity_phone_"+contact.id[len(demoPrefix):], contact.id, "phone", contact.phone, contact.phone, createdAt); err != nil {
			return nil, err
		}
		for key, granted := range map[string]bool{"email_marketing": contact.emailOptIn, "sms_marketing": contact.smsOptIn} {
			if err := exec("contact_consents", `INSERT INTO contact_consents(contact_id,consent_key,granted,source,updated_at) VALUES(?,?,?,?,?)`,
				contact.id, key, granted, "demo_generator", now); err != nil {
				return nil, err
			}
		}
	}

	events := []demoEvent{
		{"demo_event_001", "demo_contact_001", "demo_page_view", 7, map[string]any{"page_path": "/products/skincare", "device": "mobile"}},
		{"demo_event_002", "demo_contact_001", "demo_purchase", 6, map[string]any{"order_id": "DEMO-1001", "amount": 3200.0, "currency": "THB"}},
		{"demo_event_003", "demo_contact_002", "demo_page_view", 4, map[string]any{"page_path": "/collections/new", "device": "desktop"}},
		{"demo_event_004", "demo_contact_002", "demo_cart_abandoned", 3, map[string]any{"amount": 1850.0, "item_count": 2}},
		{"demo_event_005", "demo_contact_003", "demo_purchase", 12, map[string]any{"order_id": "DEMO-1002", "amount": 280.0, "currency": "SGD"}},
		{"demo_event_006", "demo_contact_004", "demo_page_view", 14, map[string]any{"page_path": "/blog/routine", "device": "tablet"}},
		{"demo_event_007", "demo_contact_005", "demo_cart_abandoned", 7, map[string]any{"amount": 990.0, "item_count": 1}},
		{"demo_event_008", "demo_contact_006", "demo_page_view", 2, map[string]any{"page_path": "/products/serum", "device": "mobile"}},
		{"demo_event_009", "demo_contact_006", "demo_purchase", 1, map[string]any{"order_id": "DEMO-1003", "amount": 4900.0, "currency": "THB"}},
		{"demo_event_010", "demo_contact_006", "demo_page_view", 0, map[string]any{"page_path": "/account/orders", "device": "mobile"}},
	}
	eventNumber := len(events) + 1
	devices := []string{"mobile", "desktop", "tablet"}
	for index, contact := range contacts[6:] {
		contactNumber := index + 7
		events = append(events, demoEvent{
			id:        fmt.Sprintf("demo_event_%04d", eventNumber),
			contactID: contact.id,
			eventKey:  "demo_page_view",
			daysAgo:   contact.daysAgo,
			properties: map[string]any{
				"page_path": fmt.Sprintf("/demo/products/%d", contactNumber%12+1),
				"device":    devices[contactNumber%len(devices)],
			},
		})
		eventNumber++
		if contactNumber%3 == 0 {
			currency := map[string]string{"TH": "THB", "SG": "SGD", "US": "USD"}[contact.country]
			events = append(events, demoEvent{
				id:        fmt.Sprintf("demo_event_%04d", eventNumber),
				contactID: contact.id,
				eventKey:  "demo_purchase",
				daysAgo:   min(contact.daysAgo+1, 90),
				properties: map[string]any{
					"order_id": fmt.Sprintf("DEMO-%04d", contactNumber),
					"amount":   float64(500 + (contactNumber%25)*375),
					"currency": currency,
				},
			})
			eventNumber++
		}
		if contactNumber%5 == 0 {
			events = append(events, demoEvent{
				id:        fmt.Sprintf("demo_event_%04d", eventNumber),
				contactID: contact.id,
				eventKey:  "demo_cart_abandoned",
				daysAgo:   min(contact.daysAgo+2, 90),
				properties: map[string]any{
					"amount":     float64(750 + (contactNumber%18)*250),
					"item_count": contactNumber%5 + 1,
				},
			})
			eventNumber++
		}
	}
	for _, event := range events {
		properties, _ := json.Marshal(event.properties)
		occurredAt := seededAt.AddDate(0, 0, -event.daysAgo).Format(time.RFC3339Nano)
		if err := exec("events", `INSERT INTO events
			(id,contact_id,event_key,definition_version,occurred_at,properties_json,idempotency_key,created_at)
			VALUES(?,?,?,1,?,?,?,?)`, event.id, event.contactID, event.eventKey, occurredAt, string(properties), event.id, occurredAt); err != nil {
			return nil, err
		}
	}

	thaiRule := `{"field":"contact.demo_country","operator":"equals","value":"TH"}`
	if err := exec("segments", `INSERT INTO segments
		(id,name,description,definition_type,definition_json,status,last_validated_at,created_at,updated_at)
		VALUES(?,?,?,?,?,'validated',?,?,?)`, "demo_segment_thailand", "Demo: Thailand contacts", "Generated Thailand demo audience", "rule", thaiRule, now, now, now); err != nil {
		return nil, err
	}
	if err := exec("segments", `INSERT INTO segments
		(id,name,description,definition_type,sql_query,sql_params_json,status,last_validated_at,created_at,updated_at)
		VALUES(?,?,?,'sql',?,?,'validated',?,?,?)`, "demo_segment_high_value", "Demo: High-value contacts", "Generated contacts with lifetime value at least 50000",
		`SELECT id AS contact_id FROM contacts WHERE id GLOB 'demo_*' AND CAST(json_extract(attributes_json, '$.demo_lifetime_value') AS REAL) >= :minimum`, `{"minimum":50000}`, now, now, now); err != nil {
		return nil, err
	}
	segmentMembers := map[string][]string{
		"demo_segment_thailand":   {},
		"demo_segment_high_value": {},
	}
	for _, contact := range contacts {
		if contact.country == "TH" {
			segmentMembers["demo_segment_thailand"] = append(segmentMembers["demo_segment_thailand"], contact.id)
		}
		if contact.lifetime >= 50000 {
			segmentMembers["demo_segment_high_value"] = append(segmentMembers["demo_segment_high_value"], contact.id)
		}
	}
	for segmentID, contactIDs := range segmentMembers {
		for _, contactID := range contactIDs {
			if err := exec("segment_memberships", `INSERT INTO segment_memberships(segment_id,segment_version,contact_id,evaluated_at) VALUES(?,1,?,?)`, segmentID, contactID, now); err != nil {
				return nil, err
			}
		}
	}

	if err := exec("message_templates", `INSERT INTO message_templates(id,name,channel,subject,body,status,created_at,updated_at)
		VALUES(?,?,?,?,?,'active',?,?)`, "demo_template_email", "Demo: Gold member offer", "email", "A reward for our gold members", "Hi {{demo_first_name}}, enjoy your demo reward.", now, now); err != nil {
		return nil, err
	}
	if err := exec("message_templates", `INSERT INTO message_templates(id,name,channel,subject,body,status,created_at,updated_at)
		VALUES(?,?,? ,NULL,?,'active',?,?)`, "demo_template_sms", "Demo: Cart reminder", "sms", "Your demo cart is waiting for you.", now, now); err != nil {
		return nil, err
	}
	if err := exec("campaigns", `INSERT INTO campaigns(id,name,segment_id,template_id,status,created_at,launched_at)
		VALUES(?,?,?,?, 'completed',?,?)`, "demo_campaign_thailand", "Demo: Thailand gold-member offer", "demo_segment_thailand", "demo_template_email", now, now); err != nil {
		return nil, err
	}
	contactsByID := make(map[string]demoContact, len(contacts))
	for _, contact := range contacts {
		contactsByID[contact.id] = contact
	}
	for _, contactID := range segmentMembers["demo_segment_thailand"] {
		status := "sent"
		contact := contactsByID[contactID]
		if !contact.emailOptIn {
			status = "suppressed"
		}
		if err := exec("campaign_audience", `INSERT INTO campaign_audience(campaign_id,contact_id,status,created_at) VALUES(?,?,?,?)`, "demo_campaign_thailand", contactID, status, now); err != nil {
			return nil, err
		}
		if status == "sent" {
			messageID := "demo_message_" + contactID[len("demo_contact_"):]
			if err := exec("message_outbox", `INSERT INTO message_outbox
				(id,contact_id,campaign_id,channel,template_id,recipient,payload_json,idempotency_key,status,scheduled_at,provider_message_id,attempt,created_at,sent_at)
				VALUES(?,?,?,?,?,?,?,?,'sent',?,?,1,?,?)`, messageID, contactID, "demo_campaign_thailand", "email", "demo_template_email", contact.email,
				`{"source":"demo_generator"}`, "demo:campaign:thailand:"+contactID, now, "demo-provider-"+contactID, now, now); err != nil {
				return nil, err
			}
		}
	}

	if err := exec("journeys", `INSERT INTO journeys(id,name,description,status,timezone,created_at,updated_at)
		VALUES(?,?,?,'draft','Asia/Bangkok',?,?)`, "demo_journey_cart_recovery", "Demo: Cart recovery", "Draft demo journey; it never enrolls or sends automatically", now, now); err != nil {
		return nil, err
	}
	if err := exec("journey_versions", `INSERT INTO journey_versions(id,journey_id,version,status,created_at) VALUES(?,?,1,'draft',?)`,
		"demo_journey_version_cart_recovery", "demo_journey_cart_recovery", now); err != nil {
		return nil, err
	}
	journeySteps := []struct{ id, key, typ, name, configuration string }{
		{"demo_step_cart_trigger", "cart_trigger", "trigger", "Cart abandoned", `{"type":"event","event_key":"demo_cart_abandoned"}`},
		{"demo_step_wait", "wait", "delay", "Wait one hour", `{"duration":1,"unit":"hours"}`},
		{"demo_step_reminder", "reminder", "action", "Send reminder", `{"type":"send_message","template_id":"demo_template_sms","message_type":"marketing"}`},
		{"demo_step_end", "end", "end", "Done", `{}`},
	}
	for _, step := range journeySteps {
		if err := exec("journey_steps", `INSERT INTO journey_steps(id,journey_version_id,step_key,step_type,name,configuration_json) VALUES(?,?,?,?,?,?)`,
			step.id, "demo_journey_version_cart_recovery", step.key, step.typ, step.name, step.configuration); err != nil {
			return nil, err
		}
	}
	transitions := []struct{ id, from, to string }{
		{"demo_transition_trigger_wait", "demo_step_cart_trigger", "demo_step_wait"},
		{"demo_transition_wait_reminder", "demo_step_wait", "demo_step_reminder"},
		{"demo_transition_reminder_end", "demo_step_reminder", "demo_step_end"},
	}
	for priority, transition := range transitions {
		if err := exec("journey_transitions", `INSERT INTO journey_transitions
			(id,journey_version_id,from_step_id,outcome,to_step_id,priority) VALUES(?,? ,?,'default',?,?)`,
			transition.id, "demo_journey_version_cart_recovery", transition.from, transition.to, priority); err != nil {
			return nil, err
		}
	}
	if err := exec("audit_log", `INSERT INTO audit_log(id,action,entity_type,entity_id,details_json,created_at) VALUES(?,?,?,?,?,?)`,
		"demo_audit_generated", "demo_data_generated", "demo_dataset", "demo", `{"source":"gen_demo_data"}`, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{
		"database_id":       databaseID,
		"status":            "generated",
		"demo_prefix":       demoPrefix,
		"created":           created,
		"replaced_existing": replaced,
		"generated_at":      now,
	}, nil
}

// ResetDemoData removes every record in the reserved demo namespace while
// preserving all non-demo contacts, definitions, and marketing assets.
func (s *Service) ResetDemoData(ctx context.Context, databaseID string) (map[string]any, error) {
	db, err := s.manager.Get(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	deleted, err := resetDemoDataTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var total int64
	for _, count := range deleted {
		total += count
	}
	return map[string]any{
		"database_id":   databaseID,
		"status":        "reset",
		"demo_prefix":   demoPrefix,
		"deleted":       deleted,
		"total_deleted": total,
	}, nil
}

func resetDemoDataTx(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	deleted := make(map[string]int64)
	statements := []struct {
		table string
		sql   string
	}{
		{"message_outbox", `DELETE FROM message_outbox WHERE id GLOB 'demo_*'`},
		{"journey_step_runs", `DELETE FROM journey_step_runs WHERE id GLOB 'demo_*'`},
		{"journey_enrollments", `DELETE FROM journey_enrollments WHERE id GLOB 'demo_*'`},
		{"journey_transitions", `DELETE FROM journey_transitions WHERE id GLOB 'demo_*'`},
		{"journey_steps", `DELETE FROM journey_steps WHERE id GLOB 'demo_*'`},
		{"journey_versions", `DELETE FROM journey_versions WHERE id GLOB 'demo_*'`},
		{"journeys", `DELETE FROM journeys WHERE id GLOB 'demo_*'`},
		{"campaign_audience", `DELETE FROM campaign_audience WHERE campaign_id GLOB 'demo_*' OR contact_id GLOB 'demo_*'`},
		{"campaigns", `DELETE FROM campaigns WHERE id GLOB 'demo_*'`},
		{"segment_memberships", `DELETE FROM segment_memberships WHERE segment_id GLOB 'demo_*' OR contact_id GLOB 'demo_*'`},
		{"segments", `DELETE FROM segments WHERE id GLOB 'demo_*'`},
		{"events", `DELETE FROM events WHERE id GLOB 'demo_*'`},
		{"contact_consents", `DELETE FROM contact_consents WHERE contact_id GLOB 'demo_*'`},
		{"contact_identities", `DELETE FROM contact_identities WHERE id GLOB 'demo_*'`},
		{"contacts", `DELETE FROM contacts WHERE id GLOB 'demo_*'`},
		{"event_parameter_definitions", `DELETE FROM event_parameter_definitions WHERE id GLOB 'demo_*'`},
		{"event_definitions", `DELETE FROM event_definitions WHERE id GLOB 'demo_*'`},
		{"contact_attribute_definitions", `DELETE FROM contact_attribute_definitions WHERE id GLOB 'demo_*'`},
		{"message_templates", `DELETE FROM message_templates WHERE id GLOB 'demo_*'`},
		{"audit_log", `DELETE FROM audit_log WHERE id GLOB 'demo_*'`},
	}
	for _, statement := range statements {
		result, err := tx.ExecContext(ctx, statement.sql)
		if err != nil {
			return nil, fmt.Errorf("delete demo rows from %s: %w", statement.table, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		deleted[statement.table] = count
	}
	return deleted, nil
}

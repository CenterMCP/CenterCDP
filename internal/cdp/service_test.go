package cdp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	logpkg "github.com/rvarun11/sqlite-mcp/internal/logger"
)

func setupService(t *testing.T) (*Service, *Manager) {
	t.Helper()
	dir := t.TempDir()
	manager, err := NewManager(filepath.Join(dir, "registry.db"), filepath.Join(dir, "databases"), filepath.Join(dir, "default.db"), logpkg.NewTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return NewService(manager, logpkg.NewTestLogger()), manager
}

func defineBasicSchema(t *testing.T, service *Service, databaseID string) {
	t.Helper()
	ctx := context.Background()
	_, err := service.DefineContactAttribute(ctx, DefineContactAttributeInput{
		DatabaseID: databaseID, AttributeKey: "country", DisplayName: "Country", Description: "Customer country code", DataType: "string", Filterable: true,
	})
	if err != nil {
		t.Fatalf("DefineContactAttribute: %v", err)
	}
	_, err = service.DefineEvent(ctx, DefineEventInput{
		DatabaseID: databaseID, EventKey: "purchase", DisplayName: "Purchase", Description: "Completed purchase",
		Parameters: []EventParameterInput{{ParameterKey: "amount", DisplayName: "Amount", Description: "Purchase total", DataType: "number", Required: true, Filterable: true}},
	})
	if err != nil {
		t.Fatalf("DefineEvent: %v", err)
	}
}

func TestMultipleDatabasesAreIsolated(t *testing.T) {
	service, manager := setupService(t)
	ctx := context.Background()
	brandA, err := manager.Create(ctx, "Brand A")
	if err != nil {
		t.Fatal(err)
	}
	brandB, err := manager.Create(ctx, "Brand B")
	if err != nil {
		t.Fatal(err)
	}
	if brandA.ID == brandB.ID {
		t.Fatal("database IDs must be unique")
	}
	defineBasicSchema(t, service, brandA.ID)
	dictA, err := service.DataDictionary(ctx, brandA.ID)
	if err != nil {
		t.Fatal(err)
	}
	dictB, err := service.DataDictionary(ctx, brandB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dictA["contact_attributes"].([]map[string]any)) != 1 {
		t.Fatalf("brand A dictionary missing attribute: %#v", dictA)
	}
	if len(dictB["contact_attributes"].([]map[string]any)) != 0 {
		t.Fatalf("brand B unexpectedly shares dictionary: %#v", dictB)
	}
}

func TestPreviewSegmentsAcrossDatabasesPreservesOrigin(t *testing.T) {
	service, manager := setupService(t)
	ctx := context.Background()
	targets := make([]SegmentTargetInput, 0, 2)
	for _, name := range []string{"Brand A", "Brand B"} {
		brand, err := manager.Create(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		defineBasicSchema(t, service, brand.ID)
		if _, err := service.UpsertContact(ctx, UpsertContactInput{DatabaseID: brand.ID, Identities: []IdentityInput{{Type: "email", Value: strings.ReplaceAll(strings.ToLower(name), " ", "") + "@example.com"}}, Attributes: map[string]any{"country": "TH"}}); err != nil {
			t.Fatal(err)
		}
		segment, err := service.CreateSegment(ctx, CreateSegmentInput{DatabaseID: brand.ID, Name: "Thai contacts", Type: "rule", Rule: map[string]any{"field": "country", "operator": "equals", "value": "TH"}})
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, SegmentTargetInput{DatabaseID: brand.ID, SegmentID: segment["segment_id"].(string)})
	}
	result := service.PreviewSegmentsAcrossDatabases(ctx, MultiDatabaseSegmentPreviewInput{Targets: targets})
	if result["partial"].(bool) || result["total"].(int) != 2 {
		t.Fatalf("unexpected multi-database preview: %#v", result)
	}
}

func TestContactEventSegmentsAndCampaign(t *testing.T) {
	service, _ := setupService(t)
	ctx := context.Background()
	defineBasicSchema(t, service, DefaultDatabaseID)

	contact, err := service.UpsertContact(ctx, UpsertContactInput{
		DatabaseID: DefaultDatabaseID,
		Identities: []IdentityInput{{Type: "email", Value: "Alice@Example.com", IsPrimary: true}},
		Attributes: map[string]any{"country": "TH"},
		Consents:   map[string]bool{"email_marketing": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contact.Created {
		t.Fatal("expected a newly created contact")
	}
	duplicate, err := service.UpsertContact(ctx, UpsertContactInput{
		DatabaseID: DefaultDatabaseID,
		Identities: []IdentityInput{{Type: "email", Value: "alice@example.com"}},
		Attributes: map[string]any{"country": "TH"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ContactID != contact.ContactID || duplicate.Created {
		t.Fatalf("identity resolution failed: %#v %#v", contact, duplicate)
	}
	loaded, err := service.GetContact(ctx, GetContactInput{DatabaseID: DefaultDatabaseID, ContactID: contact.ContactID})
	if err != nil || loaded["contact_id"] != contact.ContactID {
		t.Fatalf("get contact failed: %#v %v", loaded, err)
	}
	found, err := service.SearchContacts(ctx, SearchContactsInput{DatabaseID: DefaultDatabaseID, AttributeKey: "country", Value: "TH"})
	if err != nil || found.Total != 1 || len(found.Rows) != 1 {
		t.Fatalf("search contacts failed: %#v %v", found, err)
	}

	if _, err := service.UpsertContact(ctx, UpsertContactInput{
		DatabaseID: DefaultDatabaseID, ContactID: contact.ContactID, Attributes: map[string]any{"unknown": "value"},
	}); err == nil {
		t.Fatal("undefined contact attributes must be rejected")
	}

	event, err := service.TrackEvent(ctx, TrackEventInput{
		DatabaseID: DefaultDatabaseID, ContactID: contact.ContactID, EventKey: "purchase",
		Properties: map[string]any{"amount": 1250.5}, IdempotencyKey: "purchase-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event["duplicate"].(bool) {
		t.Fatal("first event must not be a duplicate")
	}
	duplicateEvent, err := service.TrackEvent(ctx, TrackEventInput{
		DatabaseID: DefaultDatabaseID, ContactID: contact.ContactID, EventKey: "purchase",
		Properties: map[string]any{"amount": 1250.5}, IdempotencyKey: "purchase-1",
	})
	if err != nil || !duplicateEvent["duplicate"].(bool) {
		t.Fatalf("idempotent event failed: %#v %v", duplicateEvent, err)
	}

	ruleSegment, err := service.CreateSegment(ctx, CreateSegmentInput{
		DatabaseID: DefaultDatabaseID, Name: "Thai buyers", Type: "rule",
		Rule: map[string]any{"all": []any{
			map[string]any{"field": "contact.country", "operator": "equals", "value": "TH"},
			map[string]any{"event": "purchase", "operator": "occurred"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	segmentID := ruleSegment["segment_id"].(string)
	preview, err := service.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: DefaultDatabaseID, SegmentID: segmentID})
	if err != nil || preview.Count != 1 {
		t.Fatalf("rule segment preview = %#v, %v", preview, err)
	}

	sqlSegment, err := service.CreateSegment(ctx, CreateSegmentInput{
		DatabaseID: DefaultDatabaseID, Name: "SQL Thai contacts", Type: "sql",
		SQL:        `SELECT id AS contact_id FROM contacts WHERE json_extract(attributes_json, '$.country')=:country`,
		Parameters: map[string]any{"country": "TH"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlPreview, err := service.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: DefaultDatabaseID, SegmentID: sqlSegment["segment_id"].(string)})
	if err != nil || sqlPreview.Count != 1 {
		t.Fatalf("SQL segment preview = %#v, %v", sqlPreview, err)
	}

	template, err := service.SaveTemplate(ctx, SaveTemplateInput{DatabaseID: DefaultDatabaseID, Name: "Offer", Channel: "email", Subject: "Hello", Body: "Offer", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	campaign, err := service.CreateCampaign(ctx, CreateCampaignInput{DatabaseID: DefaultDatabaseID, Name: "Thai offer", SegmentID: segmentID, TemplateID: template["template_id"].(string)})
	if err != nil {
		t.Fatal(err)
	}
	launched, err := service.LaunchCampaign(ctx, CampaignInput{DatabaseID: DefaultDatabaseID, CampaignID: campaign["campaign_id"].(string)})
	if err != nil {
		t.Fatal(err)
	}
	if launched["queued"].(int) != 1 || launched["suppressed"].(int) != 0 {
		t.Fatalf("unexpected campaign launch: %#v", launched)
	}
}

func TestGenerateAndResetDemoData(t *testing.T) {
	service, manager := setupService(t)
	ctx := context.Background()
	defineBasicSchema(t, service, DefaultDatabaseID)
	userContact, err := service.UpsertContact(ctx, UpsertContactInput{
		DatabaseID: DefaultDatabaseID,
		Identities: []IdentityInput{{Type: "email", Value: "user@example.com", IsPrimary: true}},
		Attributes: map[string]any{"country": "TH"},
		Consents:   map[string]bool{"email_marketing": true},
	})
	if err != nil {
		t.Fatal(err)
	}

	generated, err := service.GenerateDemoData(ctx, DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	created := generated["created"].(map[string]int64)
	if created["contacts"] != 300 || created["events"] != 461 || created["segments"] != 2 {
		t.Fatalf("unexpected generated counts: %#v", created)
	}

	db, err := manager.Get(ctx, DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	assertCount := func(query string, want int) {
		t.Helper()
		var got int
		if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("count for %q = %d, want %d", query, got, want)
		}
	}
	assertCount(`SELECT COUNT(*) FROM contacts WHERE id GLOB 'demo_*'`, 300)
	assertCount(`SELECT COUNT(*) FROM events WHERE id GLOB 'demo_*'`, 461)
	assertCount(`SELECT COUNT(*) FROM message_outbox WHERE id GLOB 'demo_*' AND status='sent'`, 121)

	thai, err := service.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: DefaultDatabaseID, SegmentID: "demo_segment_thailand"})
	if err != nil {
		t.Fatal(err)
	}
	if thai.Count != 150 {
		t.Fatalf("Thailand demo segment count = %d, want 150", thai.Count)
	}
	highValue, err := service.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: DefaultDatabaseID, SegmentID: "demo_segment_high_value"})
	if err != nil {
		t.Fatal(err)
	}
	if highValue.Count != 212 {
		t.Fatalf("high-value demo segment count = %d, want 212", highValue.Count)
	}

	generatedAgain, err := service.GenerateDemoData(ctx, DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	replaced := generatedAgain["replaced_existing"].(map[string]int64)
	if replaced["contacts"] != 300 {
		t.Fatalf("second generation did not replace 300 contacts: %#v", replaced)
	}
	assertCount(`SELECT COUNT(*) FROM contacts WHERE id GLOB 'demo_*'`, 300)

	reset, err := service.ResetDemoData(ctx, DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	if reset["total_deleted"].(int64) == 0 {
		t.Fatalf("reset deleted no rows: %#v", reset)
	}
	for _, table := range []string{
		"contact_attribute_definitions", "event_definitions", "event_parameter_definitions", "contacts", "contact_identities",
		"events", "segments", "segment_memberships", "message_templates", "campaigns", "campaign_audience", "message_outbox",
		"journeys", "journey_versions", "journey_steps", "journey_transitions", "audit_log",
	} {
		assertCount(`SELECT COUNT(*) FROM `+table+` WHERE `+demoIDColumn(table)+` GLOB 'demo_*'`, 0)
	}
	if _, err := service.GetContact(ctx, GetContactInput{DatabaseID: DefaultDatabaseID, ContactID: userContact.ContactID}); err != nil {
		t.Fatalf("reset removed the non-demo contact: %v", err)
	}
	assertCount(`SELECT COUNT(*) FROM contact_attribute_definitions WHERE attribute_key='country'`, 1)
	assertCount(`SELECT COUNT(*) FROM event_definitions WHERE event_key='purchase'`, 1)
}

func TestCollectionListsArePaginated(t *testing.T) {
	service, _ := setupService(t)
	ctx := context.Background()
	if _, err := service.GenerateDemoData(ctx, DefaultDatabaseID); err != nil {
		t.Fatal(err)
	}

	contacts, err := service.ListContacts(ctx, PageInput{DatabaseID: DefaultDatabaseID, Page: 2, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if contacts.Total != 300 || contacts.TotalPages != 150 || contacts.Page != 2 || len(contacts.Rows) != 6 || contacts.BufferStartPage != 1 || contacts.BufferEndPage != 3 {
		t.Fatalf("unexpected contacts page: %#v", contacts)
	}
	if _, ok := contacts.Rows[0]["attributes"].(map[string]any); !ok {
		t.Fatalf("contact attributes were not decoded: %#v", contacts.Rows[0])
	}

	segments, err := service.ListSegments(ctx, StatusPageInput{DatabaseID: DefaultDatabaseID, Status: "validated", PageSize: 1})
	if err != nil || segments.Total != 2 || len(segments.Rows) != 2 || segments.BufferEndPage != 2 {
		t.Fatalf("unexpected segments page: %#v, %v", segments, err)
	}
	campaigns, err := service.ListCampaigns(ctx, StatusPageInput{DatabaseID: DefaultDatabaseID, Status: "completed"})
	if err != nil || campaigns.Total != 1 || len(campaigns.Rows) != 1 {
		t.Fatalf("unexpected campaigns page: %#v, %v", campaigns, err)
	}
	templates, err := service.ListMessageTemplates(ctx, StatusPageInput{DatabaseID: DefaultDatabaseID, Status: "active"})
	if err != nil || templates.Total != 2 {
		t.Fatalf("unexpected templates page: %#v, %v", templates, err)
	}
	journeys, err := service.ListJourneys(ctx, StatusPageInput{DatabaseID: DefaultDatabaseID, Status: "draft"})
	if err != nil || journeys.Total != 1 {
		t.Fatalf("unexpected journeys page: %#v, %v", journeys, err)
	}
	outbox, err := service.ListOutbox(ctx, OutboxListInput{DatabaseID: DefaultDatabaseID, Status: "sent", PageSize: 2})
	if err != nil || outbox.Total != 121 || len(outbox.Rows) != 6 || outbox.TotalPages != 61 || outbox.BufferEndPage != 3 {
		t.Fatalf("unexpected outbox page: %#v, %v", outbox, err)
	}

	nextBuffer, err := service.ListContacts(ctx, PageInput{DatabaseID: DefaultDatabaseID, Page: 4, PageSize: 20})
	if err != nil || len(nextBuffer.Rows) != 60 || nextBuffer.BufferStartPage != 4 || nextBuffer.BufferEndPage != 6 {
		t.Fatalf("unexpected next contacts buffer: %#v, %v", nextBuffer, err)
	}
	thaiContacts, err := service.SearchContacts(ctx, SearchContactsInput{
		DatabaseID: DefaultDatabaseID, AttributeKey: "demo_country", Value: "TH", Page: 1, PageSize: 20,
	})
	if err != nil || thaiContacts.Total != 150 || len(thaiContacts.Rows) != 60 || thaiContacts.BufferEndPage != 3 {
		t.Fatalf("unexpected contact search buffer: %#v, %v", thaiContacts, err)
	}
}

func demoIDColumn(table string) string {
	switch table {
	case "segment_memberships":
		return "segment_id"
	case "campaign_audience":
		return "campaign_id"
	default:
		return "id"
	}
}

func TestJourneyStopsFollowupAfterPurchase(t *testing.T) {
	service, manager := setupService(t)
	ctx := context.Background()
	defineBasicSchema(t, service, DefaultDatabaseID)
	template, err := service.SaveTemplate(ctx, SaveTemplateInput{DatabaseID: DefaultDatabaseID, Name: "Welcome", Channel: "email", Subject: "Welcome", Body: "Hello", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	templateID := template["template_id"].(string)
	journey, err := service.CreateJourney(ctx, CreateJourneyInput{
		DatabaseID: DefaultDatabaseID, Name: "Welcome journey", Timezone: "Asia/Bangkok",
		Steps: []JourneyStepInput{
			{Key: "start", Type: "trigger", Name: "New contact", Configuration: map[string]any{"type": "contact_created"}},
			{Key: "welcome", Type: "action", Name: "Welcome", Configuration: map[string]any{"type": "send_message", "template_id": templateID, "message_type": "marketing"}},
			{Key: "wait", Type: "delay", Name: "Wait", Configuration: map[string]any{"duration": 2.0, "unit": "days"}},
			{Key: "purchased", Type: "condition", Name: "Purchased?", Configuration: map[string]any{"type": "event_occurred", "event_key": "purchase"}},
			{Key: "followup", Type: "action", Name: "Follow up", Configuration: map[string]any{"type": "send_message", "template_id": templateID, "message_type": "marketing"}},
			{Key: "done", Type: "end", Name: "Done"},
		},
		Transitions: []JourneyTransitionInput{
			{From: "start", Outcome: "default", To: "welcome"},
			{From: "welcome", Outcome: "default", To: "wait"},
			{From: "wait", Outcome: "default", To: "purchased"},
			{From: "purchased", Outcome: "matched", To: "done"},
			{From: "purchased", Outcome: "not_matched", To: "followup"},
			{From: "followup", Outcome: "default", To: "done"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishJourney(ctx, JourneyInput{DatabaseID: DefaultDatabaseID, JourneyID: journey["journey_id"].(string)}); err != nil {
		t.Fatal(err)
	}
	contact, err := service.UpsertContact(ctx, UpsertContactInput{
		DatabaseID: DefaultDatabaseID, Identities: []IdentityInput{{Type: "email", Value: "journey@example.com"}},
		Attributes: map[string]any{"country": "TH"}, Consents: map[string]bool{"email_marketing": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	db, _ := manager.Get(ctx, DefaultDatabaseID)
	if err := service.processDueEnrollments(ctx, DefaultDatabaseID); err != nil {
		t.Fatal(err)
	}
	if err := service.processDueEnrollments(ctx, DefaultDatabaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.TrackEvent(ctx, TrackEventInput{DatabaseID: DefaultDatabaseID, ContactID: contact.ContactID, EventKey: "purchase", Properties: map[string]any{"amount": 100.0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE journey_enrollments SET next_run_at=? WHERE contact_id=?`, nowUTC(), contact.ContactID); err != nil {
		t.Fatal(err)
	}
	if err := service.processDueEnrollments(ctx, DefaultDatabaseID); err != nil {
		t.Fatal(err)
	}
	if err := service.processDueEnrollments(ctx, DefaultDatabaseID); err != nil {
		t.Fatal(err)
	}
	var messages int
	if err := db.QueryRow(`SELECT COUNT(*) FROM message_outbox WHERE contact_id=?`, contact.ContactID).Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if messages != 1 {
		t.Fatalf("expected only welcome message after purchase, got %d", messages)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM journey_enrollments WHERE contact_id=?`, contact.ContactID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("journey status = %s, want completed", status)
	}
}

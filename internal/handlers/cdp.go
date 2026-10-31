package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rvarun11/sqlite-mcp/internal/cdp"
)

type CDPHandler struct{ service *cdp.Service }

func NewCDPHandler(service *cdp.Service) *CDPHandler { return &CDPHandler{service: service} }

func decodeCDPInput[T any](request mcp.CallToolRequest) (T, error) {
	var input T
	raw, err := json.Marshal(request.Params.Arguments)
	if err != nil {
		return input, err
	}
	if string(raw) == "null" {
		return input, fmt.Errorf("arguments must be an object")
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return input, fmt.Errorf("invalid arguments: %w", err)
	}
	return input, nil
}

func cdpResult(value any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	raw, _ := json.MarshalIndent(value, "", "  ")
	return mcp.NewToolResultStructured(value, string(raw)), nil
}

func (h *CDPHandler) CreateDatabase(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.DatabaseInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.Manager().Create(ctx, in.Name)
	return cdpResult(value, err)
}
func (h *CDPHandler) ListDatabases(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	value, err := h.service.Manager().List(ctx)
	return cdpResult(value, err)
}
func (h *CDPHandler) DataDictionary(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.DatabaseScopedInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.DataDictionary(ctx, in.DatabaseID)
	return cdpResult(value, err)
}
func (h *CDPHandler) DefineContactAttribute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.DefineContactAttributeInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.DefineContactAttribute(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) DefineEvent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.DefineEventInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.DefineEvent(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) UpsertContact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.UpsertContactInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.UpsertContact(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) GetContact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.GetContactInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.GetContact(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) SearchContacts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.SearchContactsInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.SearchContacts(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) TrackEvent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.TrackEventInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.TrackEvent(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) CreateSegment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.CreateSegmentInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.CreateSegment(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) PreviewSegment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.PreviewSegmentInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.PreviewSegment(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) PreviewSegmentsAcrossDatabases(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.MultiDatabaseSegmentPreviewInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	if len(in.Targets) == 0 {
		return cdpResult(nil, fmt.Errorf("at least one segment target is required"))
	}
	return cdpResult(h.service.PreviewSegmentsAcrossDatabases(ctx, in), nil)
}
func (h *CDPHandler) SaveTemplate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.SaveTemplateInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.SaveTemplate(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) CreateCampaign(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.CreateCampaignInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.CreateCampaign(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) LaunchCampaign(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.CampaignInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.LaunchCampaign(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) CreateJourney(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.CreateJourneyInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.CreateJourney(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) PublishJourney(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.JourneyInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.PublishJourney(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) PauseJourney(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.JourneyInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.PauseJourney(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) ListOutbox(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.OutboxListInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.ListOutbox(ctx, in)
	return cdpResult(value, err)
}

func RegisterCDPTools(mcpServer *server.MCPServer, h *CDPHandler) {
	register := func(name, description string, readOnly, destructive, idempotent bool, tool mcp.Tool, handler server.ToolHandlerFunc) {
		tool.Name = name
		tool.Description = description
		tool.Annotations.ReadOnlyHint = &readOnly
		tool.Annotations.DestructiveHint = &destructive
		tool.Annotations.IdempotentHint = &idempotent
		mcpServer.AddTool(tool, handler)
	}
	register("create_customer_database", "Create an isolated customer database for a business or brand.", false, false, false, mcp.NewTool("create_customer_database", mcp.WithInputSchema[cdp.DatabaseInput]()), h.CreateDatabase)
	register("list_customer_databases", "List available customer databases. Use database_id explicitly in every CDP operation.", true, false, true, mcp.NewTool("list_customer_databases"), h.ListDatabases)
	register("get_data_dictionary", "Return contact attributes, events, and event parameters defined for a customer database.", true, false, true, mcp.NewTool("get_data_dictionary", mcp.WithInputSchema[cdp.DatabaseScopedInput]()), h.DataDictionary)
	register("define_contact_attribute", "Create or version a typed contact attribute definition before ingesting that field.", false, false, true, mcp.NewTool("define_contact_attribute", mcp.WithInputSchema[cdp.DefineContactAttributeInput]()), h.DefineContactAttribute)
	register("define_event", "Create or version an event definition and its typed parameters.", false, false, true, mcp.NewTool("define_event", mcp.WithInputSchema[cdp.DefineEventInput]()), h.DefineEvent)
	register("upsert_contact", "Resolve identities and transactionally create or update a contact using the database data dictionary.", false, false, true, mcp.NewTool("upsert_contact", mcp.WithInputSchema[cdp.UpsertContactInput]()), h.UpsertContact)
	register("get_contact", "Get a contact with attributes, identities, and consent state.", true, false, true, mcp.NewTool("get_contact", mcp.WithInputSchema[cdp.GetContactInput]()), h.GetContact)
	register("search_contacts", "Search contacts by one typed, filterable attribute.", true, false, true, mcp.NewTool("search_contacts", mcp.WithInputSchema[cdp.SearchContactsInput]()), h.SearchContacts)
	register("track_event", "Append a validated, idempotent customer event.", false, false, true, mcp.NewTool("track_event", mcp.WithInputSchema[cdp.TrackEventInput]()), h.TrackEvent)
	register("create_segment", "Create and validate a rule segment or constrained read-only SQL segment returning contact_id.", false, false, false, mcp.NewTool("create_segment", mcp.WithInputSchema[cdp.CreateSegmentInput]()), h.CreateSegment)
	register("preview_segment", "Evaluate a segment with a time and row limit, returning count and sample contact IDs.", true, false, true, mcp.NewTool("preview_segment", mcp.WithInputSchema[cdp.PreviewSegmentInput]()), h.PreviewSegment)
	register("preview_segments_across_databases", "Evaluate explicit database and segment pairs, preserving per-database results and partial errors.", true, false, true, mcp.NewTool("preview_segments_across_databases", mcp.WithInputSchema[cdp.MultiDatabaseSegmentPreviewInput]()), h.PreviewSegmentsAcrossDatabases)
	register("save_message_template", "Save an email, SMS, or push message template.", false, false, false, mcp.NewTool("save_message_template", mcp.WithInputSchema[cdp.SaveTemplateInput]()), h.SaveTemplate)
	register("create_campaign", "Create a ready campaign from a validated segment and active template.", false, false, false, mcp.NewTool("create_campaign", mcp.WithInputSchema[cdp.CreateCampaignInput]()), h.CreateCampaign)
	register("launch_campaign", "Materialize the audience, enforce marketing consent, and enqueue campaign messages.", false, true, false, mcp.NewTool("launch_campaign", mcp.WithInputSchema[cdp.CampaignInput]()), h.LaunchCampaign)
	register("create_journey", "Create and validate a versioned customer journey graph.", false, false, false, mcp.NewTool("create_journey", mcp.WithInputSchema[cdp.CreateJourneyInput]()), h.CreateJourney)
	register("publish_journey", "Publish a draft journey so matching contacts and events can enroll.", false, true, false, mcp.NewTool("publish_journey", mcp.WithInputSchema[cdp.JourneyInput]()), h.PublishJourney)
	register("pause_journey", "Pause new enrollments for an active journey.", false, true, true, mcp.NewTool("pause_journey", mcp.WithInputSchema[cdp.JourneyInput]()), h.PauseJourney)
	register("list_message_outbox", "Inspect queued, sent, failed, or suppressed messages. This server does not send to providers directly.", true, false, true, mcp.NewTool("list_message_outbox", mcp.WithInputSchema[cdp.OutboxListInput]()), h.ListOutbox)
}

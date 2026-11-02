package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rvarun11/sqlite-mcp/internal/appversion"
	"github.com/rvarun11/sqlite-mcp/internal/cdp"
)

type CDPHandler struct{ service *cdp.Service }

func NewCDPHandler(service *cdp.Service) *CDPHandler { return &CDPHandler{service: service} }

func (h *CDPHandler) GetCodeVersion(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return cdpResult(appversion.Current(), nil)
}

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

func decodeOptionalCDPInput[T any](request mcp.CallToolRequest) (T, error) {
	var input T
	if request.Params.Arguments == nil {
		return input, nil
	}
	return decodeCDPInput[T](request)
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
func (h *CDPHandler) ListDatabases(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeOptionalCDPInput[cdp.PaginationInput](request)
	if err != nil {
		return tableResult(nil, err)
	}
	value, err := h.service.ListDatabases(ctx, in)
	return tableResult(newPagedTable("Customer databases", "databases", "list_customer_databases", []tableColumn{
		{Key: "id", Label: "Database ID"}, {Key: "name", Label: "Name"}, {Key: "slug", Label: "Slug"},
		{Key: "status", Label: "Status"}, {Key: "schema_version", Label: "Schema version"}, {Key: "created_at", Label: "Created"},
	}, value, map[string]any{}), err)
}
func (h *CDPHandler) GenDemoData(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.DatabaseScopedInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.GenerateDemoData(ctx, in.DatabaseID)
	return cdpResult(value, err)
}
func (h *CDPHandler) CreateHotelSampleData(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.HotelSampleInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.CreateHotelSampleData(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) ListPopularFood(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.PageInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.ListPopularFood(ctx, in)
	return tableResult(newPagedTable("Popular restaurant dishes", "popular_food", "list_popular_food", []tableColumn{
		{Key: "dish_id", Label: "Dish ID"}, {Key: "dish_name", Label: "Dish"}, {Key: "dish_name_th", Label: "Thai name"},
		{Key: "category", Label: "Category"}, {Key: "quantity_sold", Label: "Quantity sold"}, {Key: "order_count", Label: "Orders"},
		{Key: "item_revenue", Label: "Item revenue"}, {Key: "currency", Label: "Currency"},
	}, value, map[string]any{"database_id": in.DatabaseID}), err)
}
func (h *CDPHandler) ResetDemoData(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.DatabaseScopedInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.ResetDemoData(ctx, in.DatabaseID)
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
	if err != nil {
		return tableResult(nil, err)
	}
	return tableResult(newPagedTable("Contact search results", "contacts", "search_contacts", contactColumns(value.Rows), value,
		map[string]any{"database_id": in.DatabaseID, "attribute_key": in.AttributeKey, "value": in.Value}), nil)
}

func (h *CDPHandler) ListContacts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.PageInput](request)
	if err != nil {
		return tableResult(nil, err)
	}
	value, err := h.service.ListContacts(ctx, in)
	if err != nil {
		return tableResult(nil, err)
	}
	return tableResult(newPagedTable("Contacts", "contacts", "list_contacts", contactColumns(value.Rows), value,
		map[string]any{"database_id": in.DatabaseID}), nil)
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
func (h *CDPHandler) ListSegments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.StatusPageInput](request)
	if err != nil {
		return tableResult(nil, err)
	}
	value, err := h.service.ListSegments(ctx, in)
	return tableResult(newPagedTable("Segments", "segments", "list_segments", []tableColumn{
		{Key: "segment_id", Label: "Segment ID"}, {Key: "name", Label: "Name"}, {Key: "definition_type", Label: "Type"},
		{Key: "status", Label: "Status"}, {Key: "version", Label: "Version"}, {Key: "last_validated_at", Label: "Last validated"}, {Key: "updated_at", Label: "Updated"},
	}, value, map[string]any{"database_id": in.DatabaseID, "status": in.Status}), err)
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
func (h *CDPHandler) ListMessageTemplates(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.StatusPageInput](request)
	if err != nil {
		return tableResult(nil, err)
	}
	value, err := h.service.ListMessageTemplates(ctx, in)
	return tableResult(newPagedTable("Message templates", "message_templates", "list_message_templates", []tableColumn{
		{Key: "template_id", Label: "Template ID"}, {Key: "name", Label: "Name"}, {Key: "channel", Label: "Channel"},
		{Key: "subject", Label: "Subject"}, {Key: "status", Label: "Status"}, {Key: "updated_at", Label: "Updated"},
	}, value, map[string]any{"database_id": in.DatabaseID, "status": in.Status}), err)
}
func (h *CDPHandler) CreateCampaign(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.CreateCampaignInput](request)
	if err != nil {
		return cdpResult(nil, err)
	}
	value, err := h.service.CreateCampaign(ctx, in)
	return cdpResult(value, err)
}
func (h *CDPHandler) ListCampaigns(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.StatusPageInput](request)
	if err != nil {
		return tableResult(nil, err)
	}
	value, err := h.service.ListCampaigns(ctx, in)
	return tableResult(newPagedTable("Campaigns", "campaigns", "list_campaigns", []tableColumn{
		{Key: "campaign_id", Label: "Campaign ID"}, {Key: "name", Label: "Name"}, {Key: "status", Label: "Status"},
		{Key: "segment_name", Label: "Segment"}, {Key: "template_name", Label: "Template"}, {Key: "channel", Label: "Channel"},
		{Key: "audience_count", Label: "Audience"}, {Key: "created_at", Label: "Created"}, {Key: "launched_at", Label: "Launched"},
	}, value, map[string]any{"database_id": in.DatabaseID, "status": in.Status}), err)
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
func (h *CDPHandler) ListJourneys(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, err := decodeCDPInput[cdp.StatusPageInput](request)
	if err != nil {
		return tableResult(nil, err)
	}
	value, err := h.service.ListJourneys(ctx, in)
	return tableResult(newPagedTable("Journeys", "journeys", "list_journeys", []tableColumn{
		{Key: "journey_id", Label: "Journey ID"}, {Key: "name", Label: "Name"}, {Key: "status", Label: "Status"},
		{Key: "timezone", Label: "Timezone"}, {Key: "created_at", Label: "Created"}, {Key: "updated_at", Label: "Updated"},
	}, value, map[string]any{"database_id": in.DatabaseID, "status": in.Status}), err)
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
	return tableResult(newPagedTable("Message outbox", "message_outbox", "list_message_outbox", []tableColumn{
		{Key: "id", Label: "Message ID"}, {Key: "contact_id", Label: "Contact ID"}, {Key: "campaign_id", Label: "Campaign ID"},
		{Key: "channel", Label: "Channel"}, {Key: "recipient", Label: "Recipient"}, {Key: "status", Label: "Status"},
		{Key: "scheduled_at", Label: "Scheduled"}, {Key: "attempt", Label: "Attempts"}, {Key: "last_error", Label: "Last error"},
	}, value, map[string]any{"database_id": in.DatabaseID, "status": in.Status}), err)
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
	registerTable := func(name, description string, tool mcp.Tool, handler server.ToolHandlerFunc) {
		tool.Meta = tableToolMeta()
		register(name, description, true, false, true, tool, handler)
	}
	registerTableResource(mcpServer)
	register("get_code_version", "Return the running CenterCDP feature version, exact Git revision, and whether it was built with uncommitted code changes.", true, false, true, mcp.NewTool("get_code_version"), h.GetCodeVersion)
	register("create_customer_database", "Create an isolated customer database for a business or brand.", false, false, false, mcp.NewTool("create_customer_database", mcp.WithInputSchema[cdp.DatabaseInput]()), h.CreateDatabase)
	registerTable("list_customer_databases", "List available customer databases in a paginated table. Use database_id explicitly in every CDP operation.", mcp.NewTool("list_customer_databases", mcp.WithInputSchema[cdp.PaginationInput]()), h.ListDatabases)
	register("create_hotel_sample_data", "Create Serenity Pearl Hotel sample data in an empty customer database: 80 guests, check_in, check_out and restaurant_order events, and three queryable audience lists. First create a database named Serenity Pearl Hotel using create_customer_database and pass its database_id. Optional as_of is RFC3339, defaults to now. Audiences are frozen at as_of in Asia/Bangkok. Refuses existing contacts, events, dictionary or segments; never replaces data.", false, false, false, mcp.NewTool("create_hotel_sample_data", mcp.WithInputSchema[cdp.HotelSampleInput]()), h.CreateHotelSampleData)
	registerTable("list_popular_food", "Rank dishes from completed restaurant_order events in the selected database by quantity sold, then number of orders. Reports dish names, quantity, distinct orders and item revenue excluding tax and service charges, grouped by dish_id and currency. Orders require an items array, currency and status=completed.", mcp.NewTool("list_popular_food", mcp.WithInputSchema[cdp.PageInput]()), h.ListPopularFood)
	register("gen_demo_data", "Replace the reserved demo_ namespace with a complete sample CDP dataset containing 300 varied contacts. Existing non-demo data is preserved.", false, true, true, mcp.NewTool("gen_demo_data", mcp.WithInputSchema[cdp.DatabaseScopedInput]()), h.GenDemoData)
	register("reset_demo_data", "Delete all records in the reserved demo_ namespace while preserving non-demo data.", false, true, true, mcp.NewTool("reset_demo_data", mcp.WithInputSchema[cdp.DatabaseScopedInput]()), h.ResetDemoData)
	register("get_data_dictionary", "Return contact attributes, events, and event parameters defined for a customer database.", true, false, true, mcp.NewTool("get_data_dictionary", mcp.WithInputSchema[cdp.DatabaseScopedInput]()), h.DataDictionary)
	register("define_contact_attribute", "Create or version a typed contact attribute definition before ingesting that field.", false, false, true, mcp.NewTool("define_contact_attribute", mcp.WithInputSchema[cdp.DefineContactAttributeInput]()), h.DefineContactAttribute)
	register("define_event", "Create or version an event definition and its typed parameters.", false, false, true, mcp.NewTool("define_event", mcp.WithInputSchema[cdp.DefineEventInput]()), h.DefineEvent)
	register("upsert_contact", "Resolve identities and transactionally create or update a contact using the database data dictionary.", false, false, true, mcp.NewTool("upsert_contact", mcp.WithInputSchema[cdp.UpsertContactInput]()), h.UpsertContact)
	register("get_contact", "Get a contact with attributes, identities, and consent state.", true, false, true, mcp.NewTool("get_contact", mcp.WithInputSchema[cdp.GetContactInput]()), h.GetContact)
	registerTable("list_contacts", "List contacts as a paginated table. Use this for requests to show, browse, or list contacts.", mcp.NewTool("list_contacts", mcp.WithInputSchema[cdp.PageInput]()), h.ListContacts)
	registerTable("search_contacts", "Search contacts by one typed, filterable attribute and show the results in a paginated table.", mcp.NewTool("search_contacts", mcp.WithInputSchema[cdp.SearchContactsInput]()), h.SearchContacts)
	register("track_event", "Append a validated, idempotent customer event.", false, false, true, mcp.NewTool("track_event", mcp.WithInputSchema[cdp.TrackEventInput]()), h.TrackEvent)
	register("create_segment", "Create and validate a rule segment or constrained read-only SQL segment returning contact_id.", false, false, false, mcp.NewTool("create_segment", mcp.WithInputSchema[cdp.CreateSegmentInput]()), h.CreateSegment)
	registerTable("list_segments", "List segments as a paginated table, optionally filtered by status. Use this for requests to show, browse, or list segments.", mcp.NewTool("list_segments", mcp.WithInputSchema[cdp.StatusPageInput]()), h.ListSegments)
	register("preview_segment", "Evaluate a segment with a time and row limit, returning count and sample contact IDs.", true, false, true, mcp.NewTool("preview_segment", mcp.WithInputSchema[cdp.PreviewSegmentInput]()), h.PreviewSegment)
	register("preview_segments_across_databases", "Evaluate explicit database and segment pairs, preserving per-database results and partial errors.", true, false, true, mcp.NewTool("preview_segments_across_databases", mcp.WithInputSchema[cdp.MultiDatabaseSegmentPreviewInput]()), h.PreviewSegmentsAcrossDatabases)
	register("save_message_template", "Save an email, SMS, or push message template.", false, false, false, mcp.NewTool("save_message_template", mcp.WithInputSchema[cdp.SaveTemplateInput]()), h.SaveTemplate)
	registerTable("list_message_templates", "List message templates as a paginated table, optionally filtered by status.", mcp.NewTool("list_message_templates", mcp.WithInputSchema[cdp.StatusPageInput]()), h.ListMessageTemplates)
	register("create_campaign", "Create a ready campaign from a validated segment and active template.", false, false, false, mcp.NewTool("create_campaign", mcp.WithInputSchema[cdp.CreateCampaignInput]()), h.CreateCampaign)
	registerTable("list_campaigns", "List campaigns as a paginated table, optionally filtered by status. Use this for requests to show, browse, or list campaigns.", mcp.NewTool("list_campaigns", mcp.WithInputSchema[cdp.StatusPageInput]()), h.ListCampaigns)
	register("launch_campaign", "Materialize the audience, enforce marketing consent, and enqueue campaign messages.", false, true, false, mcp.NewTool("launch_campaign", mcp.WithInputSchema[cdp.CampaignInput]()), h.LaunchCampaign)
	register("create_journey", "Create and validate a versioned customer journey graph.", false, false, false, mcp.NewTool("create_journey", mcp.WithInputSchema[cdp.CreateJourneyInput]()), h.CreateJourney)
	registerTable("list_journeys", "List customer journeys as a paginated table, optionally filtered by status.", mcp.NewTool("list_journeys", mcp.WithInputSchema[cdp.StatusPageInput]()), h.ListJourneys)
	register("publish_journey", "Publish a draft journey so matching contacts and events can enroll.", false, true, false, mcp.NewTool("publish_journey", mcp.WithInputSchema[cdp.JourneyInput]()), h.PublishJourney)
	register("pause_journey", "Pause new enrollments for an active journey.", false, true, true, mcp.NewTool("pause_journey", mcp.WithInputSchema[cdp.JourneyInput]()), h.PauseJourney)
	registerTable("list_message_outbox", "Inspect queued, sent, failed, or suppressed messages in a paginated table. This server does not send to providers directly.", mcp.NewTool("list_message_outbox", mcp.WithInputSchema[cdp.OutboxListInput]()), h.ListOutbox)
}

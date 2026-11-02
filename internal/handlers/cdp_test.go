package handlers

import (
	"context"
	"encoding/json"
	"github.com/rvarun11/sqlite-mcp/internal/cdp"
	"github.com/rvarun11/sqlite-mcp/internal/logger"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rvarun11/sqlite-mcp/internal/appversion"
)

func TestRegisterCDPTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1")
	RegisterCDPTools(mcpServer, NewCDPHandler(nil))
	tools := mcpServer.ListTools()
	for _, name := range []string{
		"get_code_version", "create_customer_database", "list_customer_databases", "gen_demo_data", "create_hotel_sample_data", "list_popular_food", "reset_demo_data", "get_data_dictionary",
		"define_contact_attribute", "define_event", "upsert_contact", "get_contact", "list_contacts", "search_contacts", "track_event",
		"create_segment", "list_segments", "preview_segment", "preview_segments_across_databases",
		"save_message_template", "list_message_templates", "create_campaign", "list_campaigns", "launch_campaign", "create_journey", "list_journeys",
		"publish_journey", "pause_journey", "list_message_outbox",
	} {
		tool := tools[name]
		if tool == nil {
			t.Errorf("tool %q was not registered", name)
			continue
		}
		if _, err := json.Marshal(tool.Tool); err != nil {
			t.Errorf("tool %q has an invalid schema: %v", name, err)
		}
	}

	for _, name := range []string{
		"list_customer_databases", "list_contacts", "search_contacts", "list_segments", "list_popular_food",
		"list_message_templates", "list_campaigns", "list_journeys", "list_message_outbox",
	} {
		tool := tools[name].Tool
		if tool.Meta == nil {
			t.Fatalf("table tool %q has no UI metadata", name)
		}
		ui, _ := tool.Meta.AdditionalFields["ui"].(map[string]any)
		if got := ui["resourceUri"]; got != tableResourceURI {
			t.Errorf("table tool %q resource URI = %v, want %q", name, got, tableResourceURI)
		}
	}

	resources := mcpServer.ListResources()
	resource := resources[tableResourceURI]
	if resource == nil {
		t.Fatalf("table resource %q was not registered", tableResourceURI)
	}
	if resource.Resource.MIMEType != tableMIMEType {
		t.Errorf("table resource MIME type = %q, want %q", resource.Resource.MIMEType, tableMIMEType)
	}
	contents, err := resource.Handler(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	html, ok := contents[0].(mcp.TextResourceContents)
	if !ok || !strings.Contains(html.Text, "tools/call") || !strings.Contains(html.Text, "Previous") || !strings.Contains(html.Text, "buffer_start_page") {
		t.Fatalf("table resource does not contain the paginated MCP Apps UI")
	}
}

func TestGetCodeVersion(t *testing.T) {
	result, err := NewCDPHandler(nil).GetCodeVersion(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := result.StructuredContent.(appversion.Info)
	if !ok {
		t.Fatalf("structured content type = %T", result.StructuredContent)
	}
	if info.FeatureVersion != appversion.FeatureVersion || info.Version == "" {
		t.Fatalf("unexpected version result: %#v", info)
	}
}

func TestCreateHotelSampleDataTool(t *testing.T) {
	dir := t.TempDir()
	manager, err := cdp.NewManager(filepath.Join(dir, "registry.db"), filepath.Join(dir, "databases"), filepath.Join(dir, "default.db"), logger.NewTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	h := NewCDPHandler(cdp.NewService(manager, logger.NewTestLogger()))
	database, err := manager.Create(context.Background(), "Serenity Pearl Hotel")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.NewMCPServer("test", "1")
	RegisterCDPTools(srv, h)
	tool := srv.ListTools()["create_hotel_sample_data"]
	var req mcp.CallToolRequest
	req.Params.Arguments = map[string]any{"database_id": database.ID, "as_of": "2026-11-03T18:00:00+07:00"}
	result, err := tool.Handler(context.Background(), req)
	if err != nil || result.IsError {
		t.Fatalf("tool failed: %v %v", result, err)
	}
	data := result.StructuredContent.(map[string]any)
	if data["contacts"] != 80 {
		t.Fatalf("unexpected result: %v", data)
	}
	var popularReq mcp.CallToolRequest
	popularReq.Params.Arguments = map[string]any{"database_id": database.ID, "page_size": 100}
	popular, err := srv.ListTools()["list_popular_food"].Handler(context.Background(), popularReq)
	if err != nil || popular.IsError {
		t.Fatalf("popular food tool failed: %v %v", popular, err)
	}
	view := popular.StructuredContent.(tableView)
	if view.View != "table" || len(view.Rows.([]map[string]any)) != 10 {
		t.Fatalf("unexpected popular food table: %+v", view)
	}
	result, err = tool.Handler(context.Background(), req)
	if err != nil || !result.IsError {
		t.Fatalf("duplicate seed not rejected: %v %v", result, err)
	}
}

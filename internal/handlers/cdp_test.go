package handlers

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterCDPTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1")
	RegisterCDPTools(mcpServer, NewCDPHandler(nil))
	tools := mcpServer.ListTools()
	for _, name := range []string{
		"create_customer_database", "list_customer_databases", "gen_demo_data", "reset_demo_data", "get_data_dictionary",
		"define_contact_attribute", "define_event", "upsert_contact", "get_contact", "search_contacts", "track_event",
		"create_segment", "preview_segment", "preview_segments_across_databases",
		"save_message_template", "create_campaign", "launch_campaign", "create_journey",
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
}

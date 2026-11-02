package cdp

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestHotelGuestIdentities(t *testing.T) {
	names, emails := map[string]bool{}, map[string]bool{}
	for n := 1; n <= 80; n++ {
		name, email := hotelGuestIdentity(n)
		if names[name] || emails[email] {
			t.Fatalf("duplicate identity %s %s", name, email)
		}
		names[name] = true
		emails[email] = true
		if !strings.Contains(name, " ") || strings.Contains(email, "guest") || !strings.Contains(email, "@example.") {
			t.Fatalf("unrealistic identity %s %s", name, email)
		}
	}
}

func TestHotelFoodDetailsAndPopularity(t *testing.T) {
	ctx := context.Background()
	s, m := setupService(t)
	database, err := m.Create(ctx, sampleHotelName)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.CreateHotelSampleData(ctx, HotelSampleInput{DatabaseID: database.ID, AsOf: "2026-11-03T18:00:00+07:00"})
	if err != nil {
		t.Fatal(err)
	}
	if result["hotel_name"] != sampleHotelName {
		t.Fatal(result)
	}
	db, err := m.Get(ctx, database.ID)
	if err != nil {
		t.Fatal(err)
	}
	var branded int
	if err = db.QueryRow(`SELECT count(*) FROM contacts c JOIN contact_identities i ON i.contact_id=c.id AND i.identity_type='email' WHERE json_extract(c.attributes_json,'$.hotel_name')=? AND json_extract(c.attributes_json,'$.email')=i.normalized_value`, sampleHotelName).Scan(&branded); err != nil || branded != 80 {
		t.Fatalf("guest branding/identity: %d %v", branded, err)
	}
	type tally struct {
		quantity, orders int64
		revenue          float64
	}
	expected := map[string]*tally{}
	rows, err := db.Query(`SELECT properties_json FROM events WHERE event_key='restaurant_order'`)
	if err != nil {
		t.Fatal(err)
	}
	orders := []map[string]any{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var order map[string]any
		if err = json.Unmarshal([]byte(raw), &order); err != nil {
			t.Fatal(err)
		}
		orders = append(orders, order)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	var definitionID string
	if err = db.QueryRow(`SELECT id FROM event_definitions WHERE event_key='restaurant_order'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	for _, order := range orders {
		if order["restaurant_name"] != "Pearl Kitchen" || order["currency"] != "THB" || order["status"] != "completed" {
			t.Fatal(order)
		}
		subtotal := 0.0
		items := order["items"].([]any)
		if len(items) < 2 {
			t.Fatal("expected itemized order")
		}
		seen := map[string]bool{}
		for _, raw := range items {
			item := raw.(map[string]any)
			id := item["dish_id"].(string)
			quantity, price, line := item["quantity"].(float64), item["unit_price"].(float64), item["line_total"].(float64)
			if item["dish_name"] == "" || item["dish_name_th"] == "" || quantity < 1 || quantity*price != line {
				t.Fatal(item)
			}
			subtotal += line
			if expected[id] == nil {
				expected[id] = &tally{}
			}
			expected[id].quantity += int64(quantity)
			expected[id].revenue += line
			if !seen[id] {
				expected[id].orders++
				seen[id] = true
			}
		}
		if subtotal != order["subtotal"].(float64) || math.Abs(subtotal+order["service_charge"].(float64)+order["tax"].(float64)-order["amount"].(float64)) > 0.001 {
			t.Fatal("receipt does not balance", order)
		}
		// Ensure generated item arrays and all receipt fields satisfy the dictionary.
		if err = validateEventProperties(ctx, db, definitionID, false, order); err != nil {
			t.Fatalf("generated receipt violates dictionary: %v", err)
		}
	}
	page, err := s.ListPopularFood(ctx, PageInput{DatabaseID: database.ID, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != len(hotelMenu) || len(page.Rows) != len(expected) {
		t.Fatalf("missing menu dishes: %v", page)
	}
	last := int64(1 << 62)
	for _, row := range page.Rows {
		id := row["dish_id"].(string)
		want := expected[id]
		if row["quantity_sold"] != want.quantity || row["order_count"] != want.orders || row["item_revenue"] != want.revenue {
			t.Fatalf("%s: %v != %+v", id, row, want)
		}
		if row["quantity_sold"].(int64) > last {
			t.Fatal("incorrect popularity ranking")
		}
		last = row["quantity_sold"].(int64)
	}
	other, err := s.ListPopularFood(ctx, PageInput{DatabaseID: "default"})
	if err != nil || other.Total != 0 {
		t.Fatalf("database isolation failed: %v %v", other, err)
	}
}

func TestPopularFoodCompletedOrdersAndCurrencies(t *testing.T) {
	s, m := setupService(t)
	ctx := context.Background()
	db, err := m.Get(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []string{
		`{"status":"completed","currency":"THB","items":[{"dish_id":"pad-thai","dish_name":"Pad Thai","quantity":2,"line_total":560},{"dish_id":"pad-thai","dish_name":"Pad Thai","quantity":1,"line_total":280}]}`,
		`{"status":"completed","currency":"USD","items":[{"dish_id":"pad-thai","quantity":1,"line_total":10}]}`,
		`{"status":"cancelled","currency":"THB","items":[{"dish_id":"pad-thai","quantity":99,"line_total":999}]}`,
		`{"status":"completed","currency":"THB","items":["bad",null,42,{"dish_id":"invalid","quantity":-1,"line_total":-100}]}`,
		`{"amount":100}`,
	}
	for _, raw := range fixtures {
		_, err = db.Exec(`INSERT INTO events(id,event_key,definition_version,occurred_at,properties_json,created_at) VALUES(?,'restaurant_order',1,'2026-11-03T12:00:00Z',?,'2026-11-03T12:00:00Z')`, newID("event_"), raw)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.ListPopularFood(ctx, PageInput{DatabaseID: "default", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.TotalPages != 2 {
		t.Fatal(page)
	}
	thb := page.Rows[0]
	if thb["currency"] != "THB" || thb["quantity_sold"] != int64(3) || thb["order_count"] != int64(1) || thb["item_revenue"] != float64(840) {
		t.Fatal(thb)
	}
}

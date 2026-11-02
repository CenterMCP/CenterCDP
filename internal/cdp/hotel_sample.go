package cdp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// HotelSampleInput seeds an empty database created with create_customer_database.
// AsOf freezes the sample audience definitions; omitted means the current time.
type HotelSampleInput struct {
	DatabaseID string `json:"database_id" jsonschema:"required"`
	AsOf       string `json:"as_of,omitempty"`
}

const hotelStaySQL = `WITH stays AS (
 SELECT c.id AS contact_id, c.attributes_json, ci.occurred_at AS check_in,
 json_extract(ci.properties_json,'$.stay_id') AS stay_id,
 (SELECT co.occurred_at FROM events co WHERE co.contact_id=c.id AND co.event_key='check_out'
 AND json_extract(co.properties_json,'$.stay_id')=json_extract(ci.properties_json,'$.stay_id')
 AND julianday(co.occurred_at)<=julianday(:as_of) ORDER BY julianday(co.occurred_at) DESC LIMIT 1) AS check_out
 FROM contacts c JOIN events ci ON ci.contact_id=c.id AND ci.event_key='check_in'
 WHERE julianday(ci.occurred_at)<=julianday(:as_of)
)`

var hotelAudiences = []struct {
	key, name, condition string
	expected             int
}{
	{"first_night", "แขกที่เข้าพักคืนแรก", `check_out IS NULL AND date(check_in,'+7 hours')=date(:as_of,'+7 hours')`, 20},
	{"no_restaurant", "แขกที่ยังพักอยู่และยังไม่ได้ใช้ร้านอาหารในการเข้าพักครั้งนี้", `check_out IS NULL AND NOT EXISTS(SELECT 1 FROM events r WHERE r.contact_id=stays.contact_id AND r.event_key='restaurant_order' AND json_extract(r.properties_json,'$.stay_id')=stays.stay_id AND julianday(r.occurred_at)>=julianday(stays.check_in) AND julianday(r.occurred_at)<=julianday(:as_of))`, 25},
	{"recent_checkout", "แขกที่เช็กเอาต์ภายใน 24 ชั่วโมง", `check_out IS NOT NULL AND julianday(check_out)>julianday(:as_of,'-24 hours')`, 15},
}

// CreateHotelSampleData atomically creates the hotel dictionary, 80 guests,
// 147 events and three frozen audiences. It never replaces existing CDP data.
func (s *Service) CreateHotelSampleData(ctx context.Context, in HotelSampleInput) (map[string]any, error) {
	asOf := time.Now().UTC()
	var err error
	if in.AsOf != "" {
		asOf, err = time.Parse(time.RFC3339, in.AsOf)
		if err != nil {
			return nil, fmt.Errorf("as_of must be RFC3339: %w", err)
		}
	}
	timestamp := asOf.Format(time.RFC3339)
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// This check and all writes share the same transaction, including validation.
	var existing int
	if err = tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM contacts)+(SELECT count(*) FROM events)+(SELECT count(*) FROM contact_attribute_definitions)+(SELECT count(*) FROM event_definitions)+(SELECT count(*) FROM segments)`).Scan(&existing); err != nil {
		return nil, err
	}
	if existing != 0 {
		return nil, fmt.Errorf("hotel sample requires an empty database; create a new customer database first")
	}
	now := nowUTC()
	exec := func(q string, args ...any) error { _, err := tx.ExecContext(ctx, q, args...); return err }
	jsonText := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	attributes := []struct{ key, typ string }{
		{"guest_name", "string"}, {"email", "string"}, {"hotel_name", "string"}, {"room_number", "string"}, {"stay_id", "string"},
		{"check_in", "datetime"}, {"check_out", "datetime"}, {"snapshot_at", "datetime"},
		{"first_night", "boolean"}, {"no_restaurant", "boolean"}, {"recent_checkout", "boolean"},
	}
	for _, a := range attributes {
		err = exec(`INSERT INTO contact_attribute_definitions(id,attribute_key,display_name,description,data_type,is_filterable,is_searchable,is_pii,created_at,updated_at) VALUES(?,?,?,?,?,1,?,?,?,?)`, newID("attr_"), a.key, a.key, "Synthetic hotel snapshot: "+a.key, a.typ, (a.key == "guest_name" || a.key == "email"), (a.key == "guest_name" || a.key == "email"), now, now)
		if err != nil {
			return nil, err
		}
	}
	for _, key := range []string{"check_in", "check_out", "restaurant_order"} {
		eventID := newID("evtdef_")
		if err = exec(`INSERT INTO event_definitions(id,event_key,display_name,description,created_at,updated_at) VALUES(?,?,?,?,?,?)`, eventID, key, key, sampleHotelName+" mock "+key, now, now); err != nil {
			return nil, err
		}
		params := []struct{ key, typ string }{{"stay_id", "string"}, {"room_number", "string"}}
		if key == "restaurant_order" {
			params = append(params, struct{ key, typ string }{"order_id", "string"}, struct{ key, typ string }{"amount", "number"},
				struct{ key, typ string }{"restaurant_name", "string"},
				struct{ key, typ string }{"table_number", "string"},
				struct{ key, typ string }{"meal_period", "string"},
				struct{ key, typ string }{"currency", "string"},
				struct{ key, typ string }{"status", "string"},
				struct{ key, typ string }{"items", "array"},
				struct{ key, typ string }{"subtotal", "number"},
				struct{ key, typ string }{"service_charge", "number"},
				struct{ key, typ string }{"tax", "number"})
		}
		for _, p := range params {
			if err = exec(`INSERT INTO event_parameter_definitions(id,event_definition_id,parameter_key,display_name,description,data_type,is_required,is_filterable) VALUES(?,?,?,?,?,?,1,1)`, newID("param_"), eventID, p.key, p.key, "Synthetic hotel "+p.key, p.typ); err != nil {
				return nil, err
			}
		}
	}
	zone := time.FixedZone("Asia/Bangkok", 7*3600)
	local := asOf.In(zone)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	firstCheckIn := midnight.Add(local.Sub(midnight) / 2)
	for n := 1; n <= 80; n++ {
		contactID := newID("contact_")
		stayID := fmt.Sprintf("SPH-STAY-%03d", n)
		room := fmt.Sprintf("%d%02d", 2+(n-1)/20, 1+(n-1)%20)
		checkIn := firstCheckIn
		if n > 20 {
			checkIn = asOf.Add(-time.Duration(48+n%4*24) * time.Hour)
		}
		var checkOut time.Time
		if n > 60 {
			checkOut = asOf.Add(-time.Duration(n-59) * time.Hour)
			if n > 75 {
				checkOut = asOf.Add(-time.Duration(48+n%4) * time.Hour)
			}
			checkIn = checkOut.Add(-72 * time.Hour)
		}
		noRestaurant := n <= 10 || n >= 21 && n <= 35
		ordered := !noRestaurant && n <= 60 || n >= 61 && n <= 72
		name, email := hotelGuestIdentity(n)
		attrs := map[string]any{"guest_name": name, "email": email, "hotel_name": sampleHotelName, "room_number": room, "stay_id": stayID, "check_in": checkIn.Format(time.RFC3339), "snapshot_at": timestamp, "first_night": n <= 20, "no_restaurant": noRestaurant, "recent_checkout": n >= 61 && n <= 75}
		if !checkOut.IsZero() {
			attrs["check_out"] = checkOut.Format(time.RFC3339)
		}
		if err = exec(`INSERT INTO contacts(id,attributes_json,created_at,updated_at) VALUES(?,?,?,?)`, contactID, jsonText(attrs), now, now); err != nil {
			return nil, err
		}
		if err = exec(`INSERT INTO contact_identities(id,contact_id,identity_type,normalized_value,original_value,is_primary,created_at) VALUES(?,?,'email',?,?,1,?)`, newID("ident_"), contactID, email, email, now); err != nil {
			return nil, err
		}
		track := func(key string, at time.Time, props map[string]any) error {
			return exec(`INSERT INTO events(id,contact_id,event_key,definition_version,occurred_at,properties_json,idempotency_key,created_at) VALUES(?,?,?,1,?,?,?,?)`, newID("event_"), contactID, key, at.Format(time.RFC3339), jsonText(props), stayID+":"+key, now)
		}
		props := func() map[string]any { return map[string]any{"stay_id": stayID, "room_number": room} }
		if err = track("check_in", checkIn, props()); err != nil {
			return nil, err
		}
		if !checkOut.IsZero() {
			if err = track("check_out", checkOut, props()); err != nil {
				return nil, err
			}
		}
		if ordered {
			order := hotelRestaurantOrder(n, stayID, room)
			end := asOf
			if !checkOut.IsZero() {
				end = checkOut
			}
			if err = track("restaurant_order", checkIn.Add(end.Sub(checkIn)/2), order); err != nil {
				return nil, err
			}
		}
	}
	result := map[string]any{"database_id": in.DatabaseID, "hotel_name": sampleHotelName, "as_of": timestamp, "timezone": "Asia/Bangkok", "contacts": 80, "in_house": 60, "events": map[string]int{"check_in": 80, "check_out": 20, "restaurant_order": 47}, "snapshot_notice": "Audience flags and segments are frozen at as_of; they do not advance with the clock."}
	lists := map[string]any{}
	for _, a := range hotelAudiences {
		query := hotelStaySQL + " SELECT DISTINCT contact_id AS contact_id FROM stays WHERE " + a.condition + " ORDER BY contact_id"
		rows, err := tx.QueryContext(ctx, query, sql.Named("as_of", timestamp))
		if err != nil {
			return nil, err
		}
		count := 0
		for rows.Next() {
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if count != a.expected {
			return nil, fmt.Errorf("hotel sample %s: expected %d contacts, got %d", a.key, a.expected, count)
		}
		segmentID := newID("segment_")
		if err = exec(`INSERT INTO segments(id,name,description,definition_type,sql_query,sql_params_json,status,last_validated_at,created_at,updated_at) VALUES(?,?,?,'sql',?,?,'validated',?,?,?)`, segmentID, a.name, "Hotel snapshot at "+timestamp, query, jsonText(map[string]any{"as_of": timestamp}), now, now, now); err != nil {
			return nil, err
		}
		lists[a.key] = map[string]any{"count": count, "segment_id": segmentID, "tool": "search_contacts", "arguments": map[string]any{"database_id": in.DatabaseID, "attribute_key": a.key, "value": true, "page": 1, "page_size": 100}}
	}
	result["lists"] = lists
	result["popular_food"] = map[string]any{"tool": "list_popular_food", "arguments": map[string]any{"database_id": in.DatabaseID, "page": 1, "page_size": 100}}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

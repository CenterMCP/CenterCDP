package cdp

import (
	"context"
	"testing"
)

func TestHotelSampleData(t *testing.T) {
	for _, asOf := range []string{"2026-11-03T18:00:00+07:00", "2026-11-03T00:00:00+07:00", "2026-11-02T17:30:00Z", ""} {
		t.Run(asOf, func(t *testing.T) {
			s, m := setupService(t)
			ctx := context.Background()
			database, err := m.Create(ctx, "Serenity Pearl Hotel")
			if err != nil {
				t.Fatal(err)
			}
			in := HotelSampleInput{DatabaseID: database.ID, AsOf: asOf}
			result, err := s.CreateHotelSampleData(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			lists := result["lists"].(map[string]any)
			for _, a := range hotelAudiences {
				list := lists[a.key].(map[string]any)
				page, err := s.SearchContacts(ctx, SearchContactsInput{DatabaseID: database.ID, AttributeKey: a.key, Value: true, PageSize: 100})
				if err != nil {
					t.Fatal(err)
				}
				if page.Total != a.expected {
					t.Fatalf("%s: %d", a.key, page.Total)
				}
				preview, err := s.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: database.ID, SegmentID: list["segment_id"].(string)})
				if err != nil {
					t.Fatal(err)
				}
				if preview.Count != a.expected {
					t.Fatalf("%s: %d", a.key, preview.Count)
				}
				ids := map[string]bool{}
				for _, r := range page.Rows {
					ids[r["contact_id"].(string)] = true
				}
				for _, id := range preview.SampleContactIDs {
					if !ids[id] {
						t.Fatalf("segment/list mismatch: %s", id)
					}
				}
			}
			db, err := m.Get(ctx, database.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				q    string
				want int
			}{
				{`SELECT count(*) FROM contacts`, 80},
				{`SELECT count(*) FROM events WHERE event_key='check_in'`, 80},
				{`SELECT count(*) FROM events WHERE event_key='check_out'`, 20},
				{`SELECT count(*) FROM events WHERE event_key='restaurant_order'`, 47},
				{`SELECT count(*) FROM contact_consents WHERE granted=1`, 0},
			} {
				var got int
				if err = db.QueryRow(tc.q).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != tc.want {
					t.Fatalf("%s: %d", tc.q, got)
				}
			}
			if _, err = s.CreateHotelSampleData(ctx, in); err == nil {
				t.Fatal("repeat seed should refuse existing data")
			}
			// A restaurant order on a previous stay must not affect this stay's audience.
			_, err = db.Exec(`INSERT INTO events(id,contact_id,event_key,definition_version,occurred_at,properties_json,created_at)
 SELECT 'previous-stay-order',id,'restaurant_order',1,'2026-01-01T12:00:00Z','{"stay_id":"previous-stay"}','2026-01-01T12:00:00Z' FROM contacts WHERE json_extract(attributes_json,'$.stay_id')='SPH-STAY-001'`)
			if err != nil {
				t.Fatal(err)
			}
			list := lists["no_restaurant"].(map[string]any)
			preview, err := s.PreviewSegment(ctx, PreviewSegmentInput{DatabaseID: database.ID, SegmentID: list["segment_id"].(string)})
			if err != nil || preview.Count != 25 {
				t.Fatalf("stay correlation: %v %v", preview, err)
			}
			defaultDB, err := m.Get(ctx, "default")
			if err != nil {
				t.Fatal(err)
			}
			var total int
			if err = defaultDB.QueryRow(`SELECT count(*) FROM contacts`).Scan(&total); err != nil || total != 0 {
				t.Fatalf("default database modified: %d %v", total, err)
			}
		})
	}
}

func TestHotelSampleRollback(t *testing.T) {
	s, m := setupService(t)
	ctx := context.Background()
	db, err := m.Get(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	// Fail after dictionaries and the first guest have been written.
	_, err = db.Exec(`CREATE TRIGGER reject_hotel_events BEFORE INSERT ON events BEGIN SELECT RAISE(ABORT,'forced failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateHotelSampleData(ctx, HotelSampleInput{DatabaseID: "default"}); err == nil {
		t.Fatal("expected forced failure")
	}
	for _, table := range []string{"contacts", "contact_identities", "events", "contact_attribute_definitions", "event_definitions", "event_parameter_definitions", "segments"} {
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial data in %s: %d", table, count)
		}
	}
	if _, err = s.CreateHotelSampleData(ctx, HotelSampleInput{DatabaseID: "default", AsOf: "bad date"}); err == nil {
		t.Fatal("invalid timestamp accepted")
	}
	if _, err = s.CreateHotelSampleData(ctx, HotelSampleInput{DatabaseID: "missing"}); err == nil {
		t.Fatal("unknown database accepted")
	}
}

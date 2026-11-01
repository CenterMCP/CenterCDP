package cdp

import "testing"

func TestPaginationWindowUsesThreePageBuffers(t *testing.T) {
	tests := []struct {
		name                             string
		page, pageSize, total            int
		wantPage, wantStart, wantEnd     int
		wantOffset, wantLimit, wantPages int
	}{
		{"first buffer", 1, 20, 300, 1, 1, 3, 0, 60, 15},
		{"end of first buffer", 3, 20, 300, 3, 1, 3, 0, 60, 15},
		{"second buffer", 4, 20, 300, 4, 4, 6, 60, 60, 15},
		{"partial final buffer", 15, 20, 300, 15, 13, 15, 240, 60, 15},
		{"page clamped", 16, 20, 300, 15, 13, 15, 240, 60, 15},
		{"page size capped", 1, 500, 300, 1, 1, 3, 0, 300, 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			window := newPaginationWindow(test.page, test.pageSize, test.total)
			if window.page != test.wantPage || window.bufferStartPage != test.wantStart || window.bufferEndPage != test.wantEnd ||
				window.offset != test.wantOffset || window.limit != test.wantLimit || window.totalPages != test.wantPages {
				t.Fatalf("unexpected window: %#v", window)
			}
		})
	}
}

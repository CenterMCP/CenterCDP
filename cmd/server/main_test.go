package main

import (
	"net"
	"testing"
)

func TestDefaultListenAddress(t *testing.T) {
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "4321")

	expected := net.JoinHostPort("127.0.0.1", "4321")
	if actual := defaultListenAddress(); actual != expected {
		t.Fatalf("defaultListenAddress() = %q, want %q", actual, expected)
	}
}

func TestDefaultListenAddressDefaults(t *testing.T) {
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")

	expected := net.JoinHostPort("0.0.0.0", "3000")
	if actual := defaultListenAddress(); actual != expected {
		t.Fatalf("defaultListenAddress() = %q, want %q", actual, expected)
	}
}

func TestNormalizeEndpointPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{name: "default", input: "/mcp", expected: "/mcp"},
		{name: "adds leading slash", input: "api/mcp", expected: "/api/mcp"},
		{name: "trims whitespace and slashes", input: "  /api/mcp/  ", expected: "/api/mcp"},
		{name: "allows root", input: "/", expected: "/"},
		{name: "rejects empty", input: " ", wantErr: true},
		{name: "rejects query", input: "/mcp?key=value", wantErr: true},
		{name: "rejects fragment", input: "/mcp#fragment", wantErr: true},
		{name: "reserves health endpoint", input: "/health", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := normalizeEndpointPath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeEndpointPath(%q) returned no error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeEndpointPath(%q) returned error: %v", tt.input, err)
			}
			if actual != tt.expected {
				t.Fatalf("normalizeEndpointPath(%q) = %q, want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

package appversion

import (
	"strings"
	"testing"
)

func TestCurrent(t *testing.T) {
	info := Current()
	if info.Name != "CenterCDP" {
		t.Fatalf("name = %q", info.Name)
	}
	if info.FeatureVersion != FeatureVersion {
		t.Fatalf("feature version = %q, want %q", info.FeatureVersion, FeatureVersion)
	}
	if !strings.HasPrefix(info.Version, FeatureVersion+"+") {
		t.Fatalf("version = %q, want prefix %q", info.Version, FeatureVersion+"+")
	}
	if info.GoVersion == "" {
		t.Fatal("Go version is empty")
	}
}

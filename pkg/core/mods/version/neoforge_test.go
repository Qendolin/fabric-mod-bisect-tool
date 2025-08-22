package version_test

import (
	"reflect"
	"testing"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods/version"
)

func TestTranslateMavenVersion(t *testing.T) {
	v, err := version.TranslateMavenVersion("1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %s", v)
	}

	_, err = version.TranslateMavenVersion("[1.0,2.0]")
	if err == nil {
		t.Fatalf("expected error for range input, got nil")
	}
}

func TestTranslateMavenVersionRange(t *testing.T) {
	cases := map[string][]string{
		"1.0":           {">=1.0"},
		"(,1.0]":        {"<=1.0"},
		"(,1.0)":        {"<1.0"},
		"[1.0]":         {"=1.0"},
		"[1.0,)":        {">=1.0"},
		"(1.0,)":        {">1.0"},
		"(1.0,2.0)":     {">1.0 <2.0"},
		"[1.0,2.0]":     {">=1.0 <=2.0"},
		"(,1.0],[1.2,)": {"<=1.0", ">=1.2"},
		"*":             {"*"},
		"[*,)":          {"*"},
		"[*,2.0]":       {"<=2.0"},
		"(1.0,*)":       {">1.0"},
	}

	for input, expected := range cases {
		res, err := version.TranslateMavenVersionRange(input)
		if err != nil {
			t.Errorf("unexpected error for %s: %v", input, err)
			continue
		}
		if !reflect.DeepEqual(res, expected) {
			t.Errorf("for %s expected %v, got %v", input, expected, res)
		}
	}
}

// Additional validation using Parse and ParseVersionPredicate
func TestParseTranslatedResults(t *testing.T) {
	_, err := version.Parse("1.2.3", false)
	if err != nil {
		t.Fatalf("unexpected error parsing version: %v", err)
	}

	pred, err := version.ParseVersionPredicate(">=1.0 <2.0")
	if err != nil {
		t.Fatalf("unexpected error parsing predicate: %v", err)
	}
	if pred == nil {
		t.Fatalf("expected non-nil predicate")
	}
}

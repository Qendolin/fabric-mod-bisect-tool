package ui

import (
	"strings"
	"testing"
)

func TestFormatModRef(t *testing.T) {
	cases := []struct {
		mod  ModViewModel
		want string
	}{
		{ModViewModel{ID: "sodium", Name: "Sodium", Version: "0.8.11"}, "Sodium (sodium 0.8.11)"},
		{ModViewModel{ID: "mystery", IsUnknown: true}, "mystery"},
	}
	for _, c := range cases {
		if got := FormatModRef(c.mod); got != c.want {
			t.Errorf("FormatModRef(%+v) = %q, want %q", c.mod, got, c.want)
		}
	}
}

func TestWriteConflictSetPlain(t *testing.T) {
	cs := ConflictSetReport{
		Mods: []CascadingDisables{
			{Mod: ModViewModel{ID: "a", Name: "Mod A", Version: "1.0", BaseFilename: "moda"}},
		},
		IfAllDisabledAlso: []ModViewModel{
			{ID: "b", Name: "Mod B", Version: "2.0", BaseFilename: "modb"},
		},
	}

	var b strings.Builder
	WriteConflictSet(&b, cs, TextStyles{ShowFile: true})
	got := b.String()

	for _, want := range []string{
		"- Mod A (a 1.0) from 'moda.jar'",
		"If you disable all mods in this conflict, you would also need to disable:",
		"- Mod B (b 2.0) from 'modb.jar'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestWriteConflictSetModsStyled(t *testing.T) {
	cs := ConflictSetReport{
		Mods: []CascadingDisables{
			{Mod: ModViewModel{ID: "a", Name: "Mod A", Version: "1.0", BaseFilename: "moda"}},
		},
	}
	styled := TextStyles{
		ModID: func(s string) string { return "[" + s + "]" },
		Muted: func(s string) string { return "<" + s + ">" },
	}

	var b strings.Builder
	WriteConflictSetMods(&b, cs.Mods, styled)
	got := b.String()

	if !strings.Contains(got, "[Mod A (a 1.0)]") {
		t.Errorf("expected the styled mod reference in output, got:\n%s", got)
	}
	if strings.Contains(got, "'moda.jar'") {
		t.Errorf("expected no file refs when ShowFile is false, got:\n%s", got)
	}
}

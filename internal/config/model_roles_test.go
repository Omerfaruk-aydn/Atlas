package config

import "testing"

func TestStripRoleReference(t *testing.T) {
	cases := map[string]string{
		"@research": "research",
		"research":  "research",
		"  @slow ":  "slow",
		"":          "",
	}
	for in, want := range cases {
		if got := StripRoleReference(in); got != want {
			t.Errorf("StripRoleReference(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveRoleEmptyName(t *testing.T) {
	c := &Config{}
	if _, ok := c.ResolveRole(""); ok {
		t.Error("expected ResolveRole(\"\") to report not found")
	}
	if _, ok := c.ResolveRole("@"); ok {
		t.Error("expected ResolveRole(\"@\") to report not found")
	}
}

func TestResolveRoleLargeAndSmall(t *testing.T) {
	c := &Config{
		Models: map[SelectedModelType]SelectedModel{
			SelectedModelTypeLarge: {Model: "gpt-5", Provider: "openai"},
			SelectedModelTypeSmall: {Model: "gpt-5-mini", Provider: "openai"},
		},
	}

	got, ok := c.ResolveRole("large")
	if !ok || got.Model != "gpt-5" {
		t.Errorf("ResolveRole(\"large\") = %+v, %v", got, ok)
	}

	got, ok = c.ResolveRole("@small")
	if !ok || got.Model != "gpt-5-mini" {
		t.Errorf("ResolveRole(\"@small\") = %+v, %v", got, ok)
	}
}

func TestResolveRoleCustom(t *testing.T) {
	c := &Config{
		Options: &Options{
			ModelRoles: map[string]SelectedModel{
				"research": {Model: "o3", Provider: "openai"},
			},
		},
	}

	got, ok := c.ResolveRole("@research")
	if !ok || got.Model != "o3" || got.Provider != "openai" {
		t.Errorf("ResolveRole(\"@research\") = %+v, %v", got, ok)
	}

	if _, ok := c.ResolveRole("frontend"); ok {
		t.Error("expected an unconfigured role to report not found")
	}
}

func TestResolveRoleNoOptions(t *testing.T) {
	c := &Config{}
	if _, ok := c.ResolveRole("research"); ok {
		t.Error("expected ResolveRole to report not found when Options is nil")
	}
}

package client

import (
	"encoding/json"
	"testing"
)

func TestParseBuildVersion(t *testing.T) {
	valid := map[string]BuildVersion{
		"0.0.0":   {0, 0, 0},
		"1.2.3":   {1, 2, 3},
		"10.0.42": {10, 0, 42},
	}
	for in, want := range valid {
		got, err := ParseBuildVersion(in)
		if err != nil {
			t.Errorf("ParseBuildVersion(%q) unexpected error: %v", in, err)
			continue
		}
		if *got != want {
			t.Errorf("ParseBuildVersion(%q) = %+v, want %+v", in, *got, want)
		}
		if got.String() != in {
			t.Errorf("String() = %q, want %q", got.String(), in)
		}
	}
	for _, in := range []string{"", "1", "1.2", "1.2.3.4", "1.2.3-rc1", "v1.2.3", "a.b.c", "1.-2.3"} {
		if _, err := ParseBuildVersion(in); err == nil {
			t.Errorf("ParseBuildVersion(%q) expected an error", in)
		}
	}
}

func TestBuildConfigVersionJSON(t *testing.T) {
	// Unset version must be omitted so Komodo keeps the current one.
	b, _ := json.Marshal(BuildConfig{})
	if string(b) != "{}" {
		t.Errorf("empty BuildConfig marshalled to %s, want {}", b)
	}
	b, _ = json.Marshal(BuildConfig{Version: &BuildVersion{1, 2, 3}})
	if want := `{"version":{"major":1,"minor":2,"patch":3}}`; string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
	var cfg BuildConfig
	if err := json.Unmarshal([]byte(`{"version":{"major":0,"minor":4,"patch":7}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version == nil || cfg.Version.String() != "0.4.7" {
		t.Errorf("unmarshalled version = %v, want 0.4.7", cfg.Version)
	}
}

func TestBuilderConfigServerIDFormats(t *testing.T) {
	cases := map[string]string{
		`{"type":"Server","params":{"server_id":"abc"}}`:          "abc", // older Komodo
		`{"type":"Server","params":{"server_ids":["abc","def"]}}`: "abc", // Komodo 2.3: list, first is primary
		`{"type":"Server","params":{"server_ids":[]}}`:            "",
		`{"type":"Server","params":{}}`:                           "",
	}
	for in, want := range cases {
		var c BuilderConfig
		if err := json.Unmarshal([]byte(in), &c); err != nil {
			t.Errorf("Unmarshal(%s): %v", in, err)
			continue
		}
		if c.ServerID != want {
			t.Errorf("Unmarshal(%s).ServerID = %q, want %q", in, c.ServerID, want)
		}
	}
	// Writes keep the single-value form, which every version accepts.
	b, _ := json.Marshal(BuilderConfig{ServerID: "abc"})
	if want := `{"type":"Server","params":{"server_id":"abc"}}`; string(b) != want {
		t.Errorf("Marshal = %s, want %s", b, want)
	}
}

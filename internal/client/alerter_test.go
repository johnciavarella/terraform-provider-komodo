package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAlerterEndpointJSON(t *testing.T) {
	cases := []struct {
		in   AlerterEndpoint
		want string
	}{
		{AlerterEndpoint{Type: "Slack", URL: "https://hooks.example/x"}, `{"type":"Slack","params":{"url":"https://hooks.example/x"}}`},
		{AlerterEndpoint{Type: "Ntfy", URL: "http://n/t", Email: "a@b.c"}, `{"type":"Ntfy","params":{"url":"http://n/t","email":"a@b.c"}}`},
		{AlerterEndpoint{Type: "Ntfy", URL: "http://n/t"}, `{"type":"Ntfy","params":{"url":"http://n/t"}}`},
		// email is never sent for endpoint types that have no such field.
		{AlerterEndpoint{Type: "Discord", URL: "http://d", Email: "a@b.c"}, `{"type":"Discord","params":{"url":"http://d"}}`},
	}
	for _, c := range cases {
		b, err := json.Marshal(c.in)
		if err != nil || string(b) != c.want {
			t.Errorf("Marshal(%+v) = %s, %v; want %s", c.in, b, err, c.want)
		}
	}

	var e AlerterEndpoint
	if err := json.Unmarshal([]byte(`{"type":"Ntfy","params":{"url":"http://n/t","email":null}}`), &e); err != nil {
		t.Fatal(err)
	}
	if e != (AlerterEndpoint{Type: "Ntfy", URL: "http://n/t"}) {
		t.Errorf("Unmarshal with null email = %+v", e)
	}
}

func TestAlerterClientRequests(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.URL.Path+" "+string(b))
		// Shape of a Komodo Resource<AlerterConfig> response.
		_, _ = w.Write([]byte(`{"_id":{"$oid":"abc123"},"name":"a1","tags":["t1"],"config":{
			"enabled":true,"endpoint":{"type":"Custom","params":{"url":"http://localhost:7000"}},
			"alert_types":["ServerCpu"],"resources":[{"type":"Stack","id":"s1"}],"except_resources":[],
			"maintenance_windows":[]}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "k", "s", "test", false)
	ctx := context.Background()

	a, err := c.CreateAlerter(ctx, "a1", AlerterConfig{Enabled: true, Endpoint: AlerterEndpoint{Type: "Custom", URL: "http://localhost:7000"}})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "abc123" || a.Name != "a1" || !a.Config.Enabled || a.Config.Endpoint.Type != "Custom" ||
		len(a.Config.AlertTypes) != 1 || len(a.Config.Resources) != 1 || a.Config.Resources[0] != (ResourceTarget{Type: "Stack", ID: "s1"}) {
		t.Errorf("decoded alerter = %+v", a)
	}
	if _, err := c.UpdateAlerter(ctx, "abc123", AlerterConfig{
		Endpoint:   AlerterEndpoint{Type: "Slack", URL: "u"},
		AlertTypes: []string{"BuildFailed"},
		Resources:  []ResourceTarget{{Type: "Server", ID: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetAlerter(ctx, "a1"); err != nil {
		t.Fatal(err)
	}
	if err := c.RenameAlerter(ctx, "abc123", "a2"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteAlerter(ctx, "abc123"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		// Empty filters go out as [] (not null, not omitted) so they clear in Komodo.
		`/write {"type":"CreateAlerter","params":{"config":{"enabled":true,"endpoint":{"type":"Custom","params":{"url":"http://localhost:7000"}},"alert_types":[],"resources":[],"except_resources":[]},"name":"a1"}}`,
		`/write {"type":"UpdateAlerter","params":{"config":{"enabled":false,"endpoint":{"type":"Slack","params":{"url":"u"}},"alert_types":["BuildFailed"],"resources":[{"type":"Server","id":"x"}],"except_resources":[]},"id":"abc123"}}`,
		`/read {"type":"GetAlerter","params":{"alerter":"a1"}}`,
		`/write {"type":"RenameAlerter","params":{"id":"abc123","name":"a2"}}`,
		`/write {"type":"DeleteAlerter","params":{"id":"abc123"}}`,
	}
	if len(bodies) != len(want) {
		t.Fatalf("got %d requests, want %d: %v", len(bodies), len(want), bodies)
	}
	for i := range want {
		if bodies[i] != want[i] {
			t.Errorf("request %d:\n got  %s\n want %s", i, bodies[i], want[i])
		}
	}
}

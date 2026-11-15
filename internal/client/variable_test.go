package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeKomodo serves the Komodo RPC shape ({type, params}) for variable calls.
func fakeKomodo(t *testing.T, vars map[string]Variable, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Type   string          `json:"type"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		if r.Header.Get("X-Api-Key") != "k" || r.Header.Get("X-Api-Secret") != "s" {
			t.Errorf("missing auth headers on %s", req.Type)
		}
		*calls = append(*calls, r.URL.Path+" "+req.Type)
		var p Variable
		_ = json.Unmarshal(req.Params, &p)
		var has map[string]json.RawMessage
		_ = json.Unmarshal(req.Params, &has)
		switch req.Type {
		case "ListVariables":
			out := []Variable{}
			for _, v := range vars {
				out = append(out, v)
			}
			_ = json.NewEncoder(w).Encode(out)
			return
		case "CreateVariable":
			vars[p.Name] = p
		case "UpdateVariableValue":
			v := vars[p.Name]
			v.Value = p.Value
			vars[p.Name] = v
		case "UpdateVariableDescription":
			v := vars[p.Name]
			v.Description = p.Description
			vars[p.Name] = v
		case "UpdateVariableIsSecret":
			if _, ok := has["is_secret"]; !ok {
				t.Errorf("UpdateVariableIsSecret sent without is_secret")
			}
			v := vars[p.Name]
			v.IsSecret = p.IsSecret
			vars[p.Name] = v
		case "DeleteVariable":
			delete(vars, p.Name)
		default:
			t.Errorf("unexpected request type %s", req.Type)
		}
		_ = json.NewEncoder(w).Encode(vars[p.Name])
	}))
}

func TestVariableClient(t *testing.T) {
	vars := map[string]Variable{}
	var calls []string
	srv := fakeKomodo(t, vars, &calls)
	defer srv.Close()
	c := NewClient(srv.URL, "k", "s", "test", false)
	ctx := context.Background()

	got, err := c.CreateVariable(ctx, Variable{Name: "A", Value: "1", Description: "d", IsSecret: true})
	if err != nil || got.Name != "A" || got.Value != "1" || !got.IsSecret {
		t.Fatalf("CreateVariable = %+v, %v", got, err)
	}
	if err := c.UpdateVariableValue(ctx, "A", "2"); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateVariableDescription(ctx, "A", "e"); err != nil {
		t.Fatal(err)
	}
	// false must be sent explicitly, not dropped as a zero value.
	if err := c.UpdateVariableIsSecret(ctx, "A", false); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetVariable(ctx, "A")
	if err != nil || *got != (Variable{Name: "A", Value: "2", Description: "e", IsSecret: false}) {
		t.Fatalf("GetVariable = %+v, %v", got, err)
	}
	if err := c.DeleteVariable(ctx, "A"); err != nil {
		t.Fatal(err)
	}
	_, err = c.GetVariable(ctx, "A")
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("GetVariable after delete: want NotFoundError, got %v", err)
	}
	want := []string{"/write CreateVariable", "/write UpdateVariableValue", "/write UpdateVariableDescription", "/write UpdateVariableIsSecret", "/read ListVariables", "/write DeleteVariable", "/read ListVariables"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v\nwant    %v", calls, want)
	}
}

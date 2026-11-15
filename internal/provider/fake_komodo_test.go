package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeKomodo is an in-memory stand-in for Komodo Core's /read, /write and
// /execute RPC endpoints. It lets Terraform run real plan/apply/import/destroy
// cycles in plain `go test`, without a live Komodo and without TF_ACC.
//
// It imitates the Komodo behaviours that matter for state handling:
//   - resources come back as {"_id": {"$oid": ...}, name, tags, config};
//   - updates are partial: only the config keys sent are replaced;
//   - a Server builder's server is returned as a `server_ids` list;
//   - procedure execution params are rewritten on save;
//   - a missing variable is an HTTP 500, not a 404.
type fakeKomodo struct {
	mu        sync.Mutex
	nextID    int
	variables map[string]map[string]interface{}
	apiKeys   map[string]map[string]interface{}
	resources map[string]map[string]map[string]interface{} // kind -> id -> resource
	requests  []string
}

var fakeKinds = []string{"Server", "Stack", "Deployment", "Builder", "Alerter", "Procedure"}

func newFakeKomodo(t *testing.T) (*fakeKomodo, *httptest.Server) {
	t.Helper()
	f := &fakeKomodo{
		variables: map[string]map[string]interface{}{},
		apiKeys:   map[string]map[string]interface{}{},
		resources: map[string]map[string]map[string]interface{}{},
	}
	for _, k := range fakeKinds {
		f.resources[k] = map[string]map[string]interface{}{}
	}
	srv := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeKomodo) find(kind, idOrName string) map[string]interface{} {
	for id, r := range f.resources[kind] {
		if id == idOrName || r["name"] == idOrName {
			return r
		}
	}
	return nil
}

// config returns the stored config of the only resource of the given kind.
// Callers must hold f.mu.
func (f *fakeKomodo) config(kind string) map[string]interface{} {
	for _, r := range f.resources[kind] {
		c, _ := r["config"].(map[string]interface{})
		return c
	}
	return nil
}

// rewriteStages mimics Komodo's save-time normalisation of execution params:
// a `stack` name becomes an ID and missing defaults are filled in.
func rewriteStages(config map[string]interface{}) {
	stages, _ := config["stages"].([]interface{})
	for _, s := range stages {
		execs, _ := s.(map[string]interface{})["executions"].([]interface{})
		for _, e := range execs {
			execution, _ := e.(map[string]interface{})["execution"].(map[string]interface{})
			params, _ := execution["params"].(map[string]interface{})
			if params == nil {
				continue
			}
			if stack, ok := params["stack"].(string); ok {
				if !strings.HasPrefix(stack, "id-") {
					params["stack"] = "id-" + stack
				}
				if _, ok := params["services"]; !ok {
					params["services"] = []interface{}{}
				}
				if _, ok := params["stop_time"]; !ok {
					params["stop_time"] = nil
				}
			}
		}
	}
}

// normalize applies the per-kind rewrites Komodo performs when saving config.
func normalize(kind string, config map[string]interface{}) {
	switch kind {
	case "Procedure":
		rewriteStages(config)
	case "Builder":
		// Komodo 2.3 stores and returns a list of servers.
		if params, ok := config["params"].(map[string]interface{}); ok {
			if id, ok := params["server_id"].(string); ok {
				delete(params, "server_id")
				params["server_ids"] = []interface{}{id}
				if id == "" {
					params["server_ids"] = []interface{}{}
				}
			}
		}
	}
}

func (f *fakeKomodo) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var req struct {
		Type   string                 `json:"type"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad body"}`, http.StatusBadRequest)
		return
	}
	f.requests = append(f.requests, req.Type)
	p := req.Params
	str := func(k string) string { s, _ := p[k].(string); return s }
	reply := func(v interface{}) { _ = json.NewEncoder(w).Encode(v) }
	notFound := func() { http.Error(w, `{"error":"not found"}`, http.StatusNotFound) }

	// Executions (deploy, destroy, start, ...) are accepted and ignored.
	if r.URL.Path == "/execute" {
		reply(map[string]interface{}{})
		return
	}

	for _, kind := range fakeKinds {
		switch req.Type {
		case "Create" + kind:
			f.nextID++
			id := fmt.Sprintf("%024x", f.nextID)
			config, _ := p["config"].(map[string]interface{})
			if config == nil {
				config = map[string]interface{}{}
			}
			if kind == "Server" {
				// Komodo's default when the field is not sent.
				if _, ok := config["send_version_mismatch_alerts"]; !ok {
					config["send_version_mismatch_alerts"] = true
				}
			}
			normalize(kind, config)
			res := map[string]interface{}{"_id": map[string]string{"$oid": id}, "name": str("name"), "tags": []interface{}{}, "config": config}
			f.resources[kind][id] = res
			reply(res)
			return
		case "Get" + kind:
			res := f.find(kind, str(strings.ToLower(kind)))
			if res == nil {
				notFound()
				return
			}
			reply(res)
			return
		case "List" + kind + "s":
			out := []interface{}{}
			for _, res := range f.resources[kind] {
				out = append(out, res)
			}
			reply(out)
			return
		case "Update" + kind:
			res := f.find(kind, str("id"))
			if res == nil {
				notFound()
				return
			}
			config, _ := p["config"].(map[string]interface{})
			normalize(kind, config)
			existing := res["config"].(map[string]interface{})
			for k, v := range config {
				existing[k] = v
			}
			reply(res)
			return
		case "Rename" + kind:
			res := f.find(kind, str("id"))
			if res == nil {
				notFound()
				return
			}
			res["name"] = str("name")
			reply(map[string]interface{}{})
			return
		case "Delete" + kind:
			res := f.find(kind, str("id"))
			if res == nil {
				notFound()
				return
			}
			delete(f.resources[kind], str("id"))
			reply(res)
			return
		}
	}

	switch req.Type {
	case "ListTags", "ListStackServices":
		reply([]interface{}{})
	case "UpdateResourceMeta":
		target, _ := p["target"].(map[string]interface{})
		kind, _ := target["type"].(string)
		id, _ := target["id"].(string)
		res := f.find(kind, id)
		if res == nil {
			notFound()
			return
		}
		res["tags"] = p["tags"]
		reply(map[string]interface{}{})
	case "CreateApiKeyForServiceUser":
		f.nextID++
		key := fmt.Sprintf("K-%d", f.nextID)
		f.apiKeys[key] = p
		reply(map[string]string{"key": key, "secret": fmt.Sprintf("S-%d", f.nextID)})
	case "DeleteApiKeyForServiceUser":
		delete(f.apiKeys, str("key"))
		reply(map[string]interface{}{})
	case "ListVariables":
		out := []interface{}{}
		for _, v := range f.variables {
			out = append(out, v)
		}
		reply(out)
	case "CreateVariable":
		f.variables[str("name")] = p
		reply(p)
	case "UpdateVariableValue", "UpdateVariableDescription", "UpdateVariableIsSecret":
		v, ok := f.variables[str("name")]
		if !ok {
			http.Error(w, `{"error":"no variable found with given name"}`, http.StatusInternalServerError)
			return
		}
		for k, val := range p {
			v[k] = val
		}
		reply(v)
	case "DeleteVariable":
		v := f.variables[str("name")]
		delete(f.variables, str("name"))
		reply(v)
	default:
		http.Error(w, fmt.Sprintf(`{"error":"fake komodo: unhandled request %s"}`, req.Type), http.StatusBadRequest)
	}
}

func (f *fakeKomodo) providerConfig(url string) string {
	return fmt.Sprintf(`
provider "komodo" {
  base_url   = %q
  api_key    = "test-key"
  api_secret = "test-secret"
}
`, url)
}

// checkEmpty fails if anything is left in the fake after destroy.
func (f *fakeKomodo) checkEmpty(_ *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		n := len(f.variables) + len(f.apiKeys)
		for _, byID := range f.resources {
			n += len(byID)
		}
		if n != 0 {
			return fmt.Errorf("%d objects left in fake Komodo after destroy", n)
		}
		return nil
	}
}

// check runs fn against the fake's stored state as a Terraform check.
func (f *fakeKomodo) check(fn func() error) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		return fn()
	}
}

func requireTerraform(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform CLI not found on PATH; skipping lifecycle test")
	}
}

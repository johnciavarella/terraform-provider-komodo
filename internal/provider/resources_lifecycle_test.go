package provider_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestLifecycleVariable(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(body string) string { return f.providerConfig(srv.URL) + body }

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(`
resource "komodo_variable" "v" {
  name  = "DOCKER_ROOT"
  value = "/opt/docker"
}
data "komodo_variable" "v" {
  name = komodo_variable.v.name
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_variable.v", "id", "DOCKER_ROOT"),
					resource.TestCheckResourceAttr("komodo_variable.v", "value", "/opt/docker"),
					resource.TestCheckResourceAttr("komodo_variable.v", "description", ""),
					resource.TestCheckResourceAttr("komodo_variable.v", "is_secret", "false"),
					resource.TestCheckResourceAttr("data.komodo_variable.v", "value", "/opt/docker"),
				),
			},
			{ // update every mutable field in place
				Config: cfg(`
resource "komodo_variable" "v" {
  name        = "DOCKER_ROOT"
  value       = "/srv/docker"
  description = "base dir"
  is_secret   = true
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_variable.v", "value", "/srv/docker"),
					resource.TestCheckResourceAttr("komodo_variable.v", "description", "base dir"),
					resource.TestCheckResourceAttr("komodo_variable.v", "is_secret", "true"),
				),
			},
			{ResourceName: "komodo_variable.v", ImportState: true, ImportStateVerify: true},
			{ // a new name replaces the variable
				Config: cfg(`
resource "komodo_variable" "v" {
  name  = "DOCKER_HOME"
  value = "/srv/docker"
}`),
				Check: resource.TestCheckResourceAttr("komodo_variable.v", "id", "DOCKER_HOME"),
			},
		},
	})
}

func TestLifecycleVariableDeletedOutOfBand(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	config := f.providerConfig(srv.URL) + `
resource "komodo_variable" "v" {
  name  = "GONE"
  value = "x"
}`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{ // removed in the Komodo UI: the next plan must offer to recreate it
				PreConfig: func() {
					f.mu.Lock()
					delete(f.variables, "GONE")
					f.mu.Unlock()
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{Config: config}, // recreate, then destroy must succeed
		},
	})
}

func TestLifecycleAlerter(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(body string) string { return f.providerConfig(srv.URL) + body }

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{ // minimal: every optional attribute unset
				Config: cfg(`
resource "komodo_alerter" "a" {
  name = "discord"
  endpoint = {
    type = "Discord"
    url  = "https://discord.example/webhook"
  }
}
data "komodo_alerter" "a" {
  id = komodo_alerter.a.name
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_alerter.a", "id"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "enabled", "true"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "endpoint.type", "Discord"),
					resource.TestCheckNoResourceAttr("komodo_alerter.a", "alert_types.#"),
					resource.TestCheckNoResourceAttr("komodo_alerter.a", "resources.#"),
					resource.TestCheckResourceAttr("data.komodo_alerter.a", "endpoint_type", "Discord"),
					resource.TestCheckResourceAttr("data.komodo_alerter.a", "alert_types.#", "0"),
				),
			},
			{ // rename, switch endpoint, add filters and tags
				Config: cfg(`
resource "komodo_alerter" "a" {
  name    = "ntfy"
  enabled = false
  endpoint = {
    type  = "Ntfy"
    url   = "https://ntfy.example/komodo"
    email = "ops@example.com"
  }
  alert_types      = ["ServerUnreachable", "BuildFailed"]
  resources        = [{ type = "Server", id = "srv1" }]
  except_resources = [{ type = "Stack", id = "stk1" }, { type = "Stack", id = "stk2" }]
  tags             = ["prod"]
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_alerter.a", "name", "ntfy"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "enabled", "false"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "endpoint.email", "ops@example.com"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "alert_types.#", "2"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "resources.0.id", "srv1"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "except_resources.#", "2"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "tags.0", "prod"),
				),
			},
			{ResourceName: "komodo_alerter.a", ImportState: true, ImportStateVerify: true},
			{ // explicit empty lists clear the filters and stay []
				Config: cfg(`
resource "komodo_alerter" "a" {
  name = "ntfy"
  endpoint = {
    type = "Ntfy"
    url  = "https://ntfy.example/komodo"
  }
  alert_types      = []
  resources        = []
  except_resources = []
  tags             = []
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_alerter.a", "alert_types.#", "0"),
					resource.TestCheckResourceAttr("komodo_alerter.a", "resources.#", "0"),
					resource.TestCheckNoResourceAttr("komodo_alerter.a", "endpoint.email"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						for _, a := range f.resources["Alerter"] {
							c := a["config"].(map[string]interface{})
							if n := len(c["alert_types"].([]interface{})) + len(c["resources"].([]interface{})) + len(c["except_resources"].([]interface{})); n != 0 {
								return fmt.Errorf("filters were not cleared in Komodo: %v", c)
							}
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAlerterValidation(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cases := map[string]struct{ body, wantErr string }{
		"endpoint type": {`endpoint = { type = "Teams", url = "u" }`, `Invalid endpoint type`},
		"email":         {`endpoint = { type = "Slack", url = "u", email = "a@b.c" }`, `email is only supported for Ntfy`},
		"alert type": {`endpoint = { type = "Slack", url = "u" }
  alert_types = ["ServerOnFire"]`, `Invalid alert type`},
		"resource type": {`endpoint = { type = "Slack", url = "u" }
  resources = [{ type = "Container", id = "x" }]`, `Invalid resource type`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      f.providerConfig(srv.URL) + "resource \"komodo_alerter\" \"a\" {\n  name = \"a\"\n  " + c.body + "\n}",
					ExpectError: regexp.MustCompile(c.wantErr),
				}},
			})
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("invalid configs reached the API: %v", f.requests)
	}
}

func TestLifecycleProcedure(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(body string) string { return f.providerConfig(srv.URL) + body }

	withStages := `
resource "komodo_procedure" "p" {
  name            = "nightly"
  schedule_format = "Cron"
  schedule        = "0 0 2 * * ?"
  failure_alert   = false

  stages = [
    {
      name = "deploy"
      executions = [
        { type = "DeployStack", params = jsonencode({ stack = "app" }) },
        { type = "BackupCoreDatabase" },
      ]
    },
    {
      name       = "wait"
      enabled    = false
      executions = [{ type = "Sleep", params = jsonencode({ duration_ms = 1000 }), enabled = false }]
    },
  ]
  tags = ["system"]
}`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{ // minimal
				Config: cfg(`
resource "komodo_procedure" "p" {
  name = "nightly"
}
data "komodo_procedure" "p" {
  id = komodo_procedure.p.id
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_procedure.p", "id"),
					resource.TestCheckNoResourceAttr("komodo_procedure.p", "stages.#"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "schedule_format", "English"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "schedule_enabled", "true"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "webhook_enabled", "true"),
					resource.TestCheckResourceAttr("data.komodo_procedure.p", "stage_names.#", "0"),
				),
			},
			{ // Komodo rewrites the DeployStack params; state keeps the configured JSON
				Config: cfg(withStages),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_procedure.p", "stages.#", "2"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "stages.0.enabled", "true"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "stages.0.executions.0.params", `{"stack":"app"}`),
					resource.TestCheckResourceAttr("komodo_procedure.p", "stages.0.executions.1.params", `{}`),
					resource.TestCheckResourceAttr("komodo_procedure.p", "stages.1.enabled", "false"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "stages.1.executions.0.enabled", "false"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "failure_alert", "false"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "tags.0", "system"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						for _, p := range f.resources["Procedure"] {
							b, _ := json.Marshal(p["config"])
							if !strings.Contains(string(b), `"stack":"id-app"`) {
								return fmt.Errorf("fake did not rewrite params; test is not exercising the rewrite: %s", b)
							}
						}
						return nil
					},
				),
			},
			{ // refresh with unchanged config: the rewrite must not show as drift
				Config:   cfg(withStages),
				PlanOnly: true,
			},
			{ // a stage edited in the Komodo UI must show as drift
				PreConfig: func() {
					f.mu.Lock()
					for _, p := range f.resources["Procedure"] {
						stages := p["config"].(map[string]interface{})["stages"].([]interface{})
						stages[0].(map[string]interface{})["name"] = "edited in UI"
					}
					f.mu.Unlock()
				},
				Config:             cfg(withStages),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // apply puts the configured stages back
				Config: cfg(withStages),
				Check:  resource.TestCheckResourceAttr("komodo_procedure.p", "stages.0.name", "deploy"),
			},
			{
				ResourceName: "komodo_procedure.p", ImportState: true, ImportStateVerify: true,
				// Imported stages carry Komodo's rewritten params, not the configured JSON.
				ImportStateVerifyIgnore: []string{"stages"},
			},
			{ // remove stages and rename
				Config: cfg(`
resource "komodo_procedure" "p" {
  name   = "nightly-renamed"
  stages = []
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_procedure.p", "name", "nightly-renamed"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "stages.#", "0"),
					resource.TestCheckResourceAttr("komodo_procedure.p", "schedule", ""),
				),
			},
		},
	})
}

func TestProcedureValidation(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cases := map[string]struct{ body, wantErr string }{
		"schedule format":   {`schedule_format = "Crontab"`, `Invalid schedule format`},
		"params not json":   {`stages = [{ name = "s", executions = [{ type = "Sleep", params = "duration_ms=1" }] }]`, `Invalid execution params`},
		"params not object": {`stages = [{ name = "s", executions = [{ type = "Sleep", params = "[1]" }] }]`, `Invalid execution params`},
		"empty type":        {`stages = [{ name = "s", executions = [{ type = "" }] }]`, `Missing execution type`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      f.providerConfig(srv.URL) + "resource \"komodo_procedure\" \"p\" {\n  name = \"p\"\n  " + c.body + "\n}",
					ExpectError: regexp.MustCompile(c.wantErr),
				}},
			})
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("invalid configs reached the API: %v", f.requests)
	}
}

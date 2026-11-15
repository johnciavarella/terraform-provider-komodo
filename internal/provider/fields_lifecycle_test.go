package provider_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestLifecycleServerFields(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(body string) string { return f.providerConfig(srv.URL) + body }
	windows := func() []interface{} { w, _ := f.config("Server")["maintenance_windows"].([]interface{}); return w }

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{ // nothing new configured: Komodo's defaults are read back, nothing is forced
				Config: cfg(`
resource "komodo_server" "s" {
  name    = "s1"
  address = "https://10.0.0.1:8120"
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_server.s", "send_version_mismatch_alerts", "true"),
					resource.TestCheckNoResourceAttr("komodo_server.s", "maintenance_windows.#"),
				),
			},
			{ // windows added in the Komodo UI are left alone and are not drift
				PreConfig: func() {
					f.mu.Lock()
					f.config("Server")["maintenance_windows"] = []interface{}{map[string]interface{}{
						"name": "ui window", "description": "", "schedule_type": "Daily", "day_of_week": "", "date": "",
						"hour": 1, "minute": 0, "duration_minutes": 30, "timezone": "", "enabled": true,
					}}
					f.mu.Unlock()
				},
				Config: cfg(`
resource "komodo_server" "s" {
  name    = "s1"
  address = "https://10.0.0.2:8120"
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("komodo_server.s", "maintenance_windows.#"),
					f.check(func() error {
						if len(windows()) != 1 {
							return fmt.Errorf("an unrelated update removed UI-managed windows: %v", windows())
						}
						return nil
					}),
				),
			},
			{ // manage them from config
				Config: cfg(`
resource "komodo_server" "s" {
  name                         = "s1"
  address                      = "https://10.0.0.2:8120"
  send_version_mismatch_alerts = false
  maintenance_windows = [
    { name = "nightly backup", hour = 2, minute = 30, duration_minutes = 60 },
    {
      name             = "weekly patching"
      description      = "os updates"
      schedule_type    = "Weekly"
      day_of_week      = "Sunday"
      hour             = 4
      duration_minutes = 120
      timezone         = "America/New_York"
      enabled          = false
    },
  ]
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_server.s", "send_version_mismatch_alerts", "false"),
					resource.TestCheckResourceAttr("komodo_server.s", "maintenance_windows.#", "2"),
					resource.TestCheckResourceAttr("komodo_server.s", "maintenance_windows.0.schedule_type", "Daily"),
					resource.TestCheckResourceAttr("komodo_server.s", "maintenance_windows.0.enabled", "true"),
					resource.TestCheckResourceAttr("komodo_server.s", "maintenance_windows.1.day_of_week", "Sunday"),
					resource.TestCheckResourceAttr("komodo_server.s", "maintenance_windows.1.enabled", "false"),
					f.check(func() error {
						b, _ := json.Marshal(windows())
						want := `[{"date":"","day_of_week":"","description":"","duration_minutes":60,"enabled":true,"hour":2,"minute":30,"name":"nightly backup","schedule_type":"Daily","timezone":""},` +
							`{"date":"","day_of_week":"Sunday","description":"os updates","duration_minutes":120,"enabled":false,"hour":4,"minute":0,"name":"weekly patching","schedule_type":"Weekly","timezone":"America/New_York"}]`
						if string(b) != want {
							return fmt.Errorf("windows sent to Komodo:\n got  %s\n want %s", b, want)
						}
						return nil
					}),
				),
			},
			{ResourceName: "komodo_server.s", ImportState: true, ImportStateVerify: true,
				// Not read on import: the attribute is only tracked once configured.
				ImportStateVerifyIgnore: []string{"passkey", "maintenance_windows"}},
			{ // an explicit empty list clears them
				Config: cfg(`
resource "komodo_server" "s" {
  name                = "s1"
  address             = "https://10.0.0.2:8120"
  maintenance_windows = []
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_server.s", "maintenance_windows.#", "0"),
					// dropped from config: the last value is kept, not reset
					resource.TestCheckResourceAttr("komodo_server.s", "send_version_mismatch_alerts", "false"),
					f.check(func() error {
						if len(windows()) != 0 {
							return fmt.Errorf("windows were not cleared in Komodo: %v", windows())
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestLifecycleStackFields(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(body string) string {
		return f.providerConfig(srv.URL) + `
resource "komodo_stack" "s" {
  name      = "app"
  server_id = "srv"
  started   = false
  deployed  = false
` + body + "\n}"
	}
	stored := func(key string) []interface{} { v, _ := f.config("Stack")[key].([]interface{}); return v }

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("komodo_stack.s", "config_files.#"),
					resource.TestCheckNoResourceAttr("komodo_stack.s", "auto_update_skip_services.#"),
				),
			},
			{ // set in the Komodo UI: left alone, and an unrelated change does not wipe them
				PreConfig: func() {
					f.mu.Lock()
					f.config("Stack")["auto_update_skip_services"] = []interface{}{"db"}
					f.config("Stack")["config_files"] = []interface{}{map[string]interface{}{"path": "ui.conf"}}
					f.mu.Unlock()
				},
				Config: cfg(`  branch = "dev"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("komodo_stack.s", "auto_update_skip_services.#"),
					resource.TestCheckNoResourceAttr("komodo_stack.s", "config_files.#"),
					f.check(func() error {
						if len(stored("auto_update_skip_services")) != 1 || len(stored("config_files")) != 1 {
							return fmt.Errorf("an unrelated update changed UI-managed lists: %v", f.config("Stack"))
						}
						return nil
					}),
				),
			},
			{
				Config: cfg(`  branch = "dev"
  auto_update_skip_services = ["db", "cache"]
  config_files = [
    { path = "nginx.conf", services = ["web"], requires = "Restart" },
    { path = "app.toml", requires = "Redeploy" },
    { path = "notes.txt" },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_stack.s", "auto_update_skip_services.#", "2"),
					resource.TestCheckResourceAttr("komodo_stack.s", "config_files.#", "3"),
					resource.TestCheckResourceAttr("komodo_stack.s", "config_files.0.services.0", "web"),
					resource.TestCheckResourceAttr("komodo_stack.s", "config_files.1.requires", "Redeploy"),
					resource.TestCheckResourceAttr("komodo_stack.s", "config_files.2.requires", "None"),
					f.check(func() error {
						b, _ := json.Marshal(stored("config_files"))
						want := `[{"path":"nginx.conf","requires":"Restart","services":["web"]},{"path":"app.toml","requires":"Redeploy"},{"path":"notes.txt","requires":"None"}]`
						if string(b) != want {
							return fmt.Errorf("config_files sent to Komodo:\n got  %s\n want %s", b, want)
						}
						return nil
					}),
				),
			},
			{ResourceName: "komodo_stack.s", ImportState: true, ImportStateVerify: true,
				// The two new lists are only tracked once configured. The rest are
				// fields the fake does not echo back when they were never sent.
				ImportStateVerifyIgnore: []string{"config_files", "auto_update_skip_services", "server_id", "started", "deployed",
					"file_contents", "pre_deploy_path", "pre_deploy_command", "post_deploy_path", "post_deploy_command"}},
			{ // explicit empty lists clear both
				Config: cfg(`  branch = "dev"
  auto_update_skip_services = []
  config_files              = []`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_stack.s", "auto_update_skip_services.#", "0"),
					resource.TestCheckResourceAttr("komodo_stack.s", "config_files.#", "0"),
					f.check(func() error {
						if len(stored("auto_update_skip_services"))+len(stored("config_files")) != 0 {
							return fmt.Errorf("lists were not cleared in Komodo: %v", f.config("Stack"))
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestLifecycleDeploymentFields(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(body string) string {
		return f.providerConfig(srv.URL) + `
resource "komodo_deployment" "d" {
  name      = "web"
  server_id = "srv"
  image     = "nginx"
  started   = false
  deployed  = false
` + body + "\n}"
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_deployment.d", "custom_name", ""),
					resource.TestCheckResourceAttr("komodo_deployment.d", "redeploy_on_build", "false"),
				),
			},
			{
				Config: cfg(`  custom_name       = "web-container"
  redeploy_on_build = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_deployment.d", "custom_name", "web-container"),
					resource.TestCheckResourceAttr("komodo_deployment.d", "redeploy_on_build", "true"),
					f.check(func() error {
						c := f.config("Deployment")
						if c["custom_name"] != "web-container" || c["redeploy_on_build"] != true {
							return fmt.Errorf("not sent to Komodo: %v", c)
						}
						return nil
					}),
				),
			},
			{ // removing custom_name must clear it in Komodo, not leave the old name
				Config: cfg(``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_deployment.d", "custom_name", ""),
					f.check(func() error {
						if c := f.config("Deployment"); c["custom_name"] != "" || c["redeploy_on_build"] != false {
							return fmt.Errorf("not reset in Komodo: custom_name=%v redeploy_on_build=%v", c["custom_name"], c["redeploy_on_build"])
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestLifecycleBuildFields(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	f.mu.Lock()
	fakeKinds = append(fakeKinds, "Build")
	f.resources["Build"] = map[string]map[string]interface{}{}
	f.mu.Unlock()
	t.Cleanup(func() { fakeKinds = fakeKinds[:len(fakeKinds)-1] })
	cfg := func(body string) string {
		return f.providerConfig(srv.URL) + "\nresource \"komodo_build\" \"b\" {\n  name = \"img\"\n" + body + "\n}"
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_build.b", "secret_args", ""),
					resource.TestCheckNoResourceAttr("komodo_build.b", "extra_args.#"),
					f.check(func() error {
						if _, sent := f.config("Build")["version"]; sent {
							return fmt.Errorf("version was sent although not configured")
						}
						return nil
					}),
				),
			},
			{ // Komodo bumped the version after a build: adopted, not reverted
				PreConfig: func() {
					f.mu.Lock()
					f.config("Build")["version"] = map[string]interface{}{"major": 0, "minor": 0, "patch": 7}
					f.mu.Unlock()
				},
				Config: cfg(`  secret_args = "TOKEN=abc"
  extra_args  = ["--no-cache", "--pull"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_build.b", "version", "0.0.7"),
					resource.TestCheckResourceAttr("komodo_build.b", "secret_args", "TOKEN=abc"),
					resource.TestCheckResourceAttr("komodo_build.b", "extra_args.1", "--pull"),
				),
			},
			{ // secret_args removed and extra_args = []: both must be cleared in Komodo
				Config: cfg(`  version                = "2.1.0"
  auto_increment_version = false
  extra_args             = []`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_build.b", "version", "2.1.0"),
					resource.TestCheckResourceAttr("komodo_build.b", "secret_args", ""),
					resource.TestCheckResourceAttr("komodo_build.b", "extra_args.#", "0"),
					f.check(func() error {
						c := f.config("Build")
						if args, _ := c["extra_args"].([]interface{}); c["secret_args"] != "" || len(args) != 0 {
							return fmt.Errorf("not cleared in Komodo: secret_args=%v extra_args=%v", c["secret_args"], c["extra_args"])
						}
						return nil
					}),
					f.check(func() error {
						b, _ := json.Marshal(f.config("Build")["version"])
						if string(b) != `{"major":2,"minor":1,"patch":0}` {
							return fmt.Errorf("version sent to Komodo = %s", b)
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestBuildVersionValidation(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      f.providerConfig(srv.URL) + "\nresource \"komodo_build\" \"b\" {\n  name    = \"img\"\n  version = \"v2\"\n}",
			ExpectError: regexp.MustCompile(`Invalid build version`),
		}},
	})
	if len(f.requests) != 0 {
		t.Errorf("an invalid version reached the API: %v", f.requests)
	}
}

func TestLifecycleBuilderServerIDs(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	config := f.providerConfig(srv.URL) + `
resource "komodo_builder" "b" {
  name      = "local"
  server_id = "server-abc"
}`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_builder.b", "server_id", "server-abc"),
					f.check(func() error {
						b, _ := json.Marshal(f.config("Builder"))
						if string(b) != `{"params":{"server_ids":["server-abc"]},"type":"Server"}` {
							return fmt.Errorf("fake is not returning the server_ids form: %s", b)
						}
						return nil
					}),
				),
			},
			// The fake returns server_ids, as Komodo 2.3 does. A second plan
			// must be empty: before the fix server_id read back as "".
			{Config: config, PlanOnly: true},
		},
	})
}

func TestLifecycleApiKeyExpires(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(body string) string {
		return f.providerConfig(srv.URL) + "\nresource \"komodo_api_key\" \"k\" {\n  user_id = \"user1\"\n  name    = \"ci\"\n" + body + "\n}"
	}
	var firstKey string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_api_key.k", "expires", "0"),
					func(s *terraform.State) error {
						firstKey = s.RootModule().Resources["komodo_api_key.k"].Primary.Attributes["key"]
						return nil
					},
				),
			},
			{Config: cfg(`  expires = 0`), PlanOnly: true}, // spelling out the default changes nothing
			{ // a new expiry replaces the key
				Config: cfg(`  expires = 4102444800000`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_api_key.k", "expires", "4102444800000"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["komodo_api_key.k"].Primary.Attributes["key"]; got == firstKey {
							return fmt.Errorf("key was not replaced when expires changed")
						}
						return nil
					},
					f.check(func() error {
						for _, k := range f.apiKeys {
							if k["expires"] != float64(4102444800000) {
								return fmt.Errorf("expires sent to Komodo = %v", k["expires"])
							}
						}
						if len(f.apiKeys) != 1 {
							return fmt.Errorf("old key was not deleted: %d keys", len(f.apiKeys))
						}
						return nil
					}),
				),
			},
		},
	})
}

// description and template are shared by every taggable resource.
func TestLifecycleDescriptionAndTemplate(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	f.mu.Lock()
	for _, k := range []string{"Build", "Repo"} {
		fakeKinds = append(fakeKinds, k)
		f.resources[k] = map[string]map[string]interface{}{}
	}
	f.mu.Unlock()
	t.Cleanup(func() { fakeKinds = fakeKinds[:len(fakeKinds)-2] })

	cfg := func(meta string) string {
		return f.providerConfig(srv.URL) + `
resource "komodo_server" "x" {
  name    = "srv"
  address = "https://10.0.0.1:8120"
` + meta + `
}
resource "komodo_stack" "x" {
  name      = "stk"
  server_id = "s"
  started   = false
  deployed  = false
` + meta + `
}
resource "komodo_deployment" "x" {
  name      = "dep"
  server_id = "s"
  image     = "nginx"
  started   = false
  deployed  = false
` + meta + `
}
resource "komodo_build" "x" {
  name = "bld"
` + meta + `
}
resource "komodo_repo" "x" {
  name = "rpo"
` + meta + `
}
resource "komodo_builder" "x" {
  name = "bdr"
` + meta + `
}`
	}
	kinds := map[string]string{"Server": "komodo_server.x", "Stack": "komodo_stack.x", "Deployment": "komodo_deployment.x",
		"Build": "komodo_build.x", "Repo": "komodo_repo.x", "Builder": "komodo_builder.x"}
	// stored returns what the fake holds for the single resource of a kind.
	stored := func(kind string) (interface{}, interface{}) {
		for _, r := range f.resources[kind] {
			return r["description"], r["template"]
		}
		return nil, nil
	}
	each := func(check func(kind, addr string) resource.TestCheckFunc) resource.TestCheckFunc {
		var checks []resource.TestCheckFunc
		for kind, addr := range kinds {
			checks = append(checks, check(kind, addr))
		}
		return resource.ComposeAggregateTestCheckFunc(checks...)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{ // not configured: never sent
				Config: cfg(``),
				Check: each(func(kind, addr string) resource.TestCheckFunc {
					return resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckNoResourceAttr(addr, "description"),
						resource.TestCheckNoResourceAttr(addr, "template"),
						f.check(func() error {
							if d, tpl := stored(kind); d != nil || tpl != nil {
								return fmt.Errorf("%s: meta sent although not configured: %v %v", kind, d, tpl)
							}
							return nil
						}),
					)
				}),
			},
			{ // set in the Komodo UI while unmanaged: not drift, and left alone
				PreConfig: func() {
					f.mu.Lock()
					for kind := range kinds {
						for _, r := range f.resources[kind] {
							r["description"] = "written in the UI"
						}
					}
					f.mu.Unlock()
				},
				Config:   cfg(``),
				PlanOnly: true,
			},
			{
				Config: cfg(`  description = "managed by terraform"
  template    = true`),
				Check: each(func(kind, addr string) resource.TestCheckFunc {
					return resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(addr, "description", "managed by terraform"),
						resource.TestCheckResourceAttr(addr, "template", "true"),
						f.check(func() error {
							if d, tpl := stored(kind); d != "managed by terraform" || tpl != true {
								return fmt.Errorf("%s: stored meta = %v %v", kind, d, tpl)
							}
							return nil
						}),
					)
				}),
			},
			{ // changed in the UI while managed: shows as drift
				PreConfig: func() {
					f.mu.Lock()
					for _, r := range f.resources["Stack"] {
						r["description"] = "edited in the UI"
					}
					f.mu.Unlock()
				},
				Config: cfg(`  description = "managed by terraform"
  template    = true`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // "" clears the description; template dropped from config is left as is
				Config: cfg(`  description = ""`),
				Check: each(func(kind, addr string) resource.TestCheckFunc {
					return resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(addr, "description", ""),
						resource.TestCheckNoResourceAttr(addr, "template"),
						f.check(func() error {
							if d, tpl := stored(kind); d != "" || tpl != true {
								return fmt.Errorf("%s: stored meta = %q %v, want \"\" true", kind, d, tpl)
							}
							return nil
						}),
					)
				}),
			},
		},
	})
}

// Values Komodo would rewrite or reject are caught at plan time, before any
// API call.
func TestFieldValidation(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	server := func(window string) string {
		return "resource \"komodo_server\" \"s\" {\n  name = \"s\"\n  address = \"https://h:8120\"\n  maintenance_windows = [" + window + "]\n}"
	}
	cases := map[string]struct{ config, wantErr string }{
		"custom_name with space": {`resource "komodo_deployment" "d" {
  name        = "d"
  image       = "nginx"
  custom_name = "my app"
}`, `Invalid custom_name`},
		"custom_name leading dash": {`resource "komodo_deployment" "d" {
  name        = "d"
  image       = "nginx"
  custom_name = "-app"
}`, `Invalid custom_name`},
		"requires lower case": {`resource "komodo_stack" "s" {
  name         = "s"
  config_files = [{ path = "a.conf", requires = "restart" }]
}`, `Invalid requires value`},
		"window hour":      {server(`{ name = "w", hour = 24, duration_minutes = 10 }`), `Invalid hour`},
		"window minute":    {server(`{ name = "w", minute = 60, duration_minutes = 10 }`), `Invalid minute`},
		"window duration":  {server(`{ name = "w", duration_minutes = 0 }`), `Invalid duration_minutes`},
		"window type":      {server(`{ name = "w", schedule_type = "Monthly", duration_minutes = 10 }`), `Invalid schedule_type`},
		"weekly no day":    {server(`{ name = "w", schedule_type = "Weekly", duration_minutes = 10 }`), `Missing day_of_week`},
		"onetime no date":  {server(`{ name = "w", schedule_type = "OneTime", duration_minutes = 10 }`), `Invalid date`},
		"onetime bad date": {server(`{ name = "w", schedule_type = "OneTime", date = "01/02/2027", duration_minutes = 10 }`), `Invalid date`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      f.providerConfig(srv.URL) + c.config,
					ExpectError: regexp.MustCompile(c.wantErr),
				}},
			})
		})
	}
	if len(f.requests) != 0 {
		t.Errorf("invalid configs reached the API: %v", f.requests)
	}
}

// A builder with several servers set in the Komodo UI keeps them when
// Terraform changes something other than server_id.
func TestLifecycleBuilderKeepsExtraServers(t *testing.T) {
	requireTerraform(t)
	f, srv := newFakeKomodo(t)
	cfg := func(name, server string) string {
		return f.providerConfig(srv.URL) + fmt.Sprintf(`
resource "komodo_builder" "b" {
  name      = %q
  server_id = %q
  tags      = ["build"]
}`, name, server)
	}
	servers := func() string {
		params, _ := f.config("Builder")["params"].(map[string]interface{})
		b, _ := json.Marshal(params["server_ids"])
		return string(b)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             f.checkEmpty(t),
		Steps: []resource.TestStep{
			{Config: cfg("builders", "srv-a")},
			{ // a second server added in the UI; Terraform only renames
				PreConfig: func() {
					f.mu.Lock()
					f.config("Builder")["params"].(map[string]interface{})["server_ids"] = []interface{}{"srv-a", "srv-b"}
					f.mu.Unlock()
				},
				Config: cfg("builders-renamed", "srv-a"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_builder.b", "name", "builders-renamed"),
					resource.TestCheckResourceAttr("komodo_builder.b", "server_id", "srv-a"),
					f.check(func() error {
						if got := servers(); got != `["srv-a","srv-b"]` {
							return fmt.Errorf("rename dropped the extra server: %s", got)
						}
						return nil
					}),
				),
			},
			{ // changing server_id does replace the list
				Config: cfg("builders-renamed", "srv-c"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_builder.b", "server_id", "srv-c"),
					f.check(func() error {
						if got := servers(); got != `["srv-c"]` {
							return fmt.Errorf("server_ids = %s, want [\"srv-c\"]", got)
						}
						return nil
					}),
				),
			},
		},
	})
}

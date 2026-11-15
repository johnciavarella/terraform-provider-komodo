package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/johnciavarella/terraform-provider-komodo/internal/provider"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"komodo": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func TestAccServerResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read.
			{
				Config: testAccServerResourceConfig(rName, "http://localhost:8120"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_server.test", "id"),
					resource.TestCheckResourceAttr("komodo_server.test", "name", rName),
					resource.TestCheckResourceAttr("komodo_server.test", "address", "http://localhost:8120"),
					resource.TestCheckResourceAttr("komodo_server.test", "enabled", "true"),
					resource.TestCheckResourceAttr("komodo_server.test", "links.0", "https://example.com"),
				),
			},
			// Import.
			{
				ResourceName:      "komodo_server.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Passkey is sensitive and won't be returned on read from some APIs.
				ImportStateVerifyIgnore: []string{"passkey"},
			},
			// Update address.
			{
				Config: testAccServerResourceConfig(rName, "http://192.168.1.100:8120"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_server.test", "address", "http://192.168.1.100:8120"),
				),
			},
		},
	})
}

func testAccServerResourceConfig(name, address string) string {
	return fmt.Sprintf(`
resource "komodo_server" "test" {
  name    = %[1]q
  address = %[2]q
  enabled = true
  links   = ["https://example.com"]
}
`, name, address)
}

func TestAccStackResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccStackResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_stack.test", "id"),
					resource.TestCheckResourceAttr("komodo_stack.test", "name", rName),
				),
			},
			{
				ResourceName:      "komodo_stack.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccStackResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "komodo_stack" "test" {
  name = %[1]q
}
`, name)
}

func TestAccDeploymentResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_deployment.test", "id"),
					resource.TestCheckResourceAttr("komodo_deployment.test", "name", rName),
					resource.TestCheckResourceAttr("komodo_deployment.test", "image", "nginx"),
				),
			},
			{
				ResourceName:      "komodo_deployment.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccDeploymentResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "komodo_deployment" "test" {
  name  = %[1]q
  image = "nginx"
}
`, name)
}

func TestAccTagResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTagResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_tag.test", "id"),
					resource.TestCheckResourceAttr("komodo_tag.test", "name", rName),
				),
			},
			{
				ResourceName:      "komodo_tag.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccTagResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "komodo_tag" "test" {
  name = %[1]q
}
`, name)
}

func TestAccBuildResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccBuildResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_build.test", "id"),
					resource.TestCheckResourceAttr("komodo_build.test", "name", rName),
				),
			},
			{
				ResourceName:      "komodo_build.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBuildResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "komodo_build" "test" {
  name = %[1]q
  repo = "myorg/test-repo"
}
`, name)
}

func TestAccRepoResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRepoResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_repo.test", "id"),
					resource.TestCheckResourceAttr("komodo_repo.test", "name", rName),
					resource.TestCheckResourceAttr("komodo_repo.test", "on_clone_path", "./"),
					resource.TestCheckResourceAttr("komodo_repo.test", "on_clone_command", "echo cloned"),
				),
			},
			{
				ResourceName:      "komodo_repo.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccRepoResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "komodo_repo" "test" {
  name             = %[1]q
  repo             = "myorg/test-repo"
  branch           = "main"
  on_clone_path    = "./"
  on_clone_command = "echo cloned"
}
`, name)
}

func TestAccVariableResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("TF_ACC_TEST")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVariableResourceConfig(rName, "first", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_variable.test", "id", rName),
					resource.TestCheckResourceAttr("komodo_variable.test", "name", rName),
					resource.TestCheckResourceAttr("komodo_variable.test", "value", "first"),
					resource.TestCheckResourceAttr("komodo_variable.test", "is_secret", "false"),
				),
			},
			{
				Config: testAccVariableResourceConfig(rName, "second", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_variable.test", "value", "second"),
					resource.TestCheckResourceAttr("komodo_variable.test", "is_secret", "true"),
				),
			},
			{
				ResourceName:      "komodo_variable.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccVariableResourceConfig(name, value string, secret bool) string {
	return fmt.Sprintf(`
resource "komodo_variable" "test" {
  name        = %[1]q
  value       = %[2]q
  description = "terraform acceptance test"
  is_secret   = %[3]t
}
`, name, value, secret)
}

func TestAccAlerterResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlerterResourceConfig(rName, "Custom", "http://localhost:7000", `["ServerUnreachable", "StackStateChange"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_alerter.test", "id"),
					resource.TestCheckResourceAttr("komodo_alerter.test", "name", rName),
					resource.TestCheckResourceAttr("komodo_alerter.test", "enabled", "false"),
					resource.TestCheckResourceAttr("komodo_alerter.test", "endpoint.type", "Custom"),
					resource.TestCheckResourceAttr("komodo_alerter.test", "alert_types.#", "2"),
				),
			},
			{
				Config: testAccAlerterResourceConfig(rName, "Ntfy", "http://localhost:8080/komodo", `[]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("komodo_alerter.test", "endpoint.type", "Ntfy"),
					resource.TestCheckResourceAttr("komodo_alerter.test", "alert_types.#", "0"),
				),
			},
			{
				ResourceName:      "komodo_alerter.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// The alerter is created disabled so the test never sends a real alert.
func testAccAlerterResourceConfig(name, endpointType, url, alertTypes string) string {
	return fmt.Sprintf(`
resource "komodo_alerter" "test" {
  name    = %[1]q
  enabled = false
  endpoint = {
    type = %[2]q
    url  = %[3]q
  }
  alert_types = %[4]s
}
`, name, endpointType, url, alertTypes)
}

func TestAccProcedureResource_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProcedureResourceConfig(rName, 1000),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("komodo_procedure.test", "id"),
					resource.TestCheckResourceAttr("komodo_procedure.test", "name", rName),
					resource.TestCheckResourceAttr("komodo_procedure.test", "stages.#", "1"),
					resource.TestCheckResourceAttr("komodo_procedure.test", "stages.0.executions.0.type", "Sleep"),
					resource.TestCheckResourceAttr("komodo_procedure.test", "schedule_format", "Cron"),
				),
			},
			{
				Config: testAccProcedureResourceConfig(rName, 2000),
				Check:  resource.TestCheckResourceAttr("komodo_procedure.test", "stages.0.executions.0.params", `{"duration_ms":2000}`),
			},
			{
				ResourceName:      "komodo_procedure.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Komodo's stored params can differ in form from the configured JSON.
				ImportStateVerifyIgnore: []string{"stages"},
			},
		},
	})
}

// The schedule is disabled so the test procedure never runs by itself.
func testAccProcedureResourceConfig(name string, sleepMs int) string {
	return fmt.Sprintf(`
resource "komodo_procedure" "test" {
  name             = %[1]q
  schedule_format  = "Cron"
  schedule         = "0 0 3 * * ?"
  schedule_enabled = false

  stages = [{
    name = "wait"
    executions = [{
      type   = "Sleep"
      params = jsonencode({ duration_ms = %[2]d })
    }]
  }]
}
`, name, sleepMs)
}

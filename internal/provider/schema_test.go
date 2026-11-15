package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestProviderSchemaValid loads every resource and data source schema through
// the provider server, which catches invalid schemas (bad defaults, missing
// nested types, duplicate type names) without needing a live Komodo.
func TestProviderSchemaValid(t *testing.T) {
	server, err := testAccProtoV6ProviderFactories["komodo"]()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("schema diagnostic: %s: %s", d.Summary, d.Detail)
	}
	for _, name := range []string{"komodo_variable"} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("resource %s is not registered", name)
		}
		if _, ok := resp.DataSourceSchemas[name]; !ok {
			t.Errorf("data source %s is not registered", name)
		}
	}
}

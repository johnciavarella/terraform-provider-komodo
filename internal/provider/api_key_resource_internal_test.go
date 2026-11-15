package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A key created before `expires` existed has no value for it in state. Taking
// on the default must not replace the key, which would rotate its secret.
func TestApiKeyExpiresReplacement(t *testing.T) {
	var resp resource.SchemaResponse
	(&ApiKeyResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	attr := resp.Schema.Attributes["expires"].(schema.Int64Attribute)
	if len(attr.PlanModifiers) != 1 {
		t.Fatalf("expected one plan modifier on expires, got %d", len(attr.PlanModifiers))
	}
	cases := []struct {
		name        string
		state, plan types.Int64
		wantReplace bool
	}{
		{"state from before the attribute", types.Int64Null(), types.Int64Value(0), false},
		{"old key given a real expiry", types.Int64Null(), types.Int64Value(4102444800000), true},
		{"unchanged", types.Int64Value(0), types.Int64Value(0), false},
		{"expiry added", types.Int64Value(0), types.Int64Value(4102444800000), true},
		{"expiry removed", types.Int64Value(4102444800000), types.Int64Value(0), true},
	}
	for _, c := range cases {
		// RequiresReplaceIf only consults the function for an existing
		// resource whose planned value differs from state, as Terraform does.
		req := planmodifier.Int64Request{StateValue: c.state, PlanValue: c.plan, ConfigValue: c.plan}
		req.State.Raw = resourceExistsMarker
		req.Plan.Raw = resourceExistsMarker
		var out planmodifier.Int64Response
		attr.PlanModifiers[0].PlanModifyInt64(context.Background(), req, &out)
		if out.Diagnostics.HasError() {
			t.Fatalf("%s: %v", c.name, out.Diagnostics)
		}
		if out.RequiresReplace != c.wantReplace {
			t.Errorf("%s: RequiresReplace = %v, want %v", c.name, out.RequiresReplace, c.wantReplace)
		}
	}
}

// resourceExistsMarker is any non-null raw value: the modifier only checks
// that state and plan are present (not a create or destroy).
var resourceExistsMarker = tftypes.NewValue(tftypes.String, "x")

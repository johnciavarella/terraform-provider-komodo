package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

func TestIsObscuredSecret(t *testing.T) {
	cases := map[string]bool{"": false, "#": true, "####": true, "ab#": false, "#a#": false, "hunter2": false}
	for in, want := range cases {
		if got := isObscuredSecret(in); got != want {
			t.Errorf("isObscuredSecret(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMapVariableToModel(t *testing.T) {
	// Admin read: the real value is returned and stored.
	m := VariableResourceModel{Value: types.StringValue("old")}
	mapVariableToModel(&client.Variable{Name: "K", Value: "new", IsSecret: true, Description: "d"}, &m)
	if m.Value.ValueString() != "new" || m.ID.ValueString() != "K" || !m.IsSecret.ValueBool() || m.Description.ValueString() != "d" {
		t.Errorf("unexpected model after admin read: %+v", m)
	}

	// Non-admin read of a secret: the mask must not replace the known value.
	m = VariableResourceModel{Value: types.StringValue("hunter2")}
	mapVariableToModel(&client.Variable{Name: "K", Value: "#######", IsSecret: true}, &m)
	if m.Value.ValueString() != "hunter2" {
		t.Errorf("masked secret overwrote state value: got %q", m.Value.ValueString())
	}

	// Import (no value in the model yet): take whatever the API returns.
	m = VariableResourceModel{Value: types.StringNull()}
	mapVariableToModel(&client.Variable{Name: "K", Value: "####", IsSecret: true}, &m)
	if m.Value.ValueString() != "####" {
		t.Errorf("import should store the API value, got %q", m.Value.ValueString())
	}

	// A non-secret value made only of '#' is a real value.
	m = VariableResourceModel{Value: types.StringValue("old")}
	mapVariableToModel(&client.Variable{Name: "K", Value: "###", IsSecret: false}, &m)
	if m.Value.ValueString() != "###" {
		t.Errorf("non-secret '#' value should be stored, got %q", m.Value.ValueString())
	}
}

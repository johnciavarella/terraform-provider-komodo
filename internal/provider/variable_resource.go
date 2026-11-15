package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

var (
	_ resource.Resource                   = &VariableResource{}
	_ resource.ResourceWithImportState    = &VariableResource{}
	_ resource.ResourceWithValidateConfig = &VariableResource{}
)

type VariableResource struct {
	client *client.Client
}

type VariableResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
	IsSecret    types.Bool   `tfsdk:"is_secret"`
}

func NewVariableResource() resource.Resource {
	return &VariableResource{}
}

func (r *VariableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_variable"
}

func (r *VariableResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo global variable. Reference it as `[[NAME]]` in the environment, build args and other interpolated fields of stacks, deployments, builds and repos. Requires an admin API key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Same as name. Variables have no separate ID in Komodo.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true, Description: "Variable name, as used in `[[NAME]]`. Komodo cannot rename a variable, so changing this replaces it.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"value": schema.StringAttribute{
				Optional: true, Computed: true, Sensitive: true,
				Default:     stringdefault.StaticString(""),
				Description: "Variable value (up to 10000 characters). Always treated as sensitive by Terraform, whether or not is_secret is set. Komodo stores it unencrypted in its database.",
			},
			"description": schema.StringAttribute{
				Optional: true, Computed: true,
				Default:     stringdefault.StaticString(""),
				Description: "Description shown in the Komodo UI.",
			},
			"is_secret": schema.BoolAttribute{
				Optional: true, Computed: true,
				Default:     booldefault.StaticBool(false),
				Description: "Mark the variable as a secret: Komodo hides its value in updates and logs, and from non-admin users.",
			},
		},
	}
}

// Komodo's own limits for variables.
var variableNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const (
	maxVariableNameLength  = 500
	maxVariableValueLength = 10000
)

func (r *VariableResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var name, value types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("value"), &value)...)
	if !name.IsNull() && !name.IsUnknown() {
		if n := name.ValueString(); !variableNamePattern.MatchString(n) || len(n) > maxVariableNameLength {
			resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid variable name",
				fmt.Sprintf("%q is not a valid variable name. It must start with a letter or underscore, contain only letters, digits and underscores, and be at most %d characters.", n, maxVariableNameLength))
		}
	}
	if !value.IsNull() && !value.IsUnknown() && utf8.RuneCountInString(value.ValueString()) > maxVariableValueLength {
		// The value itself is not echoed: it may be a secret.
		resp.Diagnostics.AddAttributeError(path.Root("value"), "Variable value too long",
			fmt.Sprintf("Komodo limits variable values to %d characters.", maxVariableValueLength))
	}
}

func (r *VariableResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	r.client = c
}

// isObscuredSecret reports whether value is the mask Komodo returns for a
// secret variable when the API key belongs to a non-admin user: '#' repeated
// to the length of the real value.
func isObscuredSecret(value string) bool {
	return value != "" && strings.Trim(value, "#") == ""
}

// mapVariableToModel copies the API variable into the model. A masked secret
// value never overwrites the value already held in the model.
func mapVariableToModel(v *client.Variable, m *VariableResourceModel) {
	m.ID = types.StringValue(v.Name)
	m.Name = types.StringValue(v.Name)
	m.Description = types.StringValue(v.Description)
	m.IsSecret = types.BoolValue(v.IsSecret)
	if v.IsSecret && isObscuredSecret(v.Value) && !m.Value.IsNull() && !m.Value.IsUnknown() {
		return
	}
	m.Value = types.StringValue(v.Value)
}

func (r *VariableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.CreateVariable(ctx, client.Variable{
		Name:        plan.Name.ValueString(),
		Value:       plan.Value.ValueString(),
		Description: plan.Description.ValueString(),
		IsSecret:    plan.IsSecret.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating variable", err.Error())
		return
	}

	mapVariableToModel(v, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VariableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// After import only id is set.
	name := state.Name.ValueString()
	if name == "" {
		name = state.ID.ValueString()
	}

	v, err := r.client.GetVariable(ctx, name)
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading variable", err.Error())
		return
	}

	mapVariableToModel(v, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VariableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state VariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()

	if !plan.Value.Equal(state.Value) {
		if err := r.client.UpdateVariableValue(ctx, name, plan.Value.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error updating variable value", err.Error())
			return
		}
	}
	if !plan.Description.Equal(state.Description) {
		if err := r.client.UpdateVariableDescription(ctx, name, plan.Description.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error updating variable description", err.Error())
			return
		}
	}
	if !plan.IsSecret.Equal(state.IsSecret) {
		if err := r.client.UpdateVariableIsSecret(ctx, name, plan.IsSecret.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Error updating variable is_secret", err.Error())
			return
		}
	}

	v, err := r.client.GetVariable(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading variable after update", err.Error())
		return
	}
	mapVariableToModel(v, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VariableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := state.Name.ValueString()

	// Komodo reports a missing variable as a generic server error, so check
	// first and treat "already gone" as success.
	if _, err := r.client.GetVariable(ctx, name); err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError("Error reading variable before delete", err.Error())
		return
	}
	if err := r.client.DeleteVariable(ctx, name); err != nil {
		resp.Diagnostics.AddError("Error deleting variable", err.Error())
	}
}

// ImportState imports by variable name.
func (r *VariableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

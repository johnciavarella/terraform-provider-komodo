package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

var _ resource.Resource = &ApiKeyResource{}

type ApiKeyResource struct {
	client *client.Client
}

type ApiKeyResourceModel struct {
	ID      types.String `tfsdk:"id"`
	UserID  types.String `tfsdk:"user_id"`
	Name    types.String `tfsdk:"name"`
	Expires types.Int64  `tfsdk:"expires"`
	Key     types.String `tfsdk:"key"`
	Secret  types.String `tfsdk:"secret"`
}

func NewApiKeyResource() resource.Resource {
	return &ApiKeyResource{}
}

func (r *ApiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *ApiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an API key for a Komodo service user. The secret is only available at creation time and is stored in Terraform state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier (same as the key value).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				Description: "ID of the service user to create the API key for. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name for the API key. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"expires": schema.Int64Attribute{
				Description: "Unix timestamp in milliseconds when the key expires. 0 (the default) means it never expires. Changing this forces a new resource.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				PlanModifiers: []planmodifier.Int64{
					// Keys created before this attribute existed have no
					// expires in state, and were created without an expiry.
					// Adopting the default (0) for them must not replace the
					// key (and so rotate its secret); asking for a real
					// expiry must, since an existing key cannot be changed.
					int64planmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.Int64Request, resp *int64planmodifier.RequiresReplaceIfFuncResponse) {
							if req.StateValue.IsNull() {
								resp.RequiresReplace = !req.PlanValue.IsUnknown() && req.PlanValue.ValueInt64() != 0
								return
							}
							resp.RequiresReplace = !req.PlanValue.Equal(req.StateValue)
						},
						"Replaces the key when expires changes.",
						"Replaces the key when expires changes.",
					),
				},
			},
			"key": schema.StringAttribute{
				Description: "The API key value (K-...). Sensitive.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"secret": schema.StringAttribute{
				Description: "The API secret value (S-...). Only available at creation time. Sensitive.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ApiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *ApiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ApiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey, err := r.client.CreateApiKeyForServiceUser(ctx, plan.UserID.ValueString(), plan.Name.ValueString(), plan.Expires.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error creating API key", err.Error())
		return
	}

	plan.ID = types.StringValue(apiKey.Key)
	plan.Key = types.StringValue(apiKey.Key)
	plan.Secret = types.StringValue(apiKey.Secret)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ApiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// The Komodo API does not provide a way to read back API key secrets, so
	// values are preserved from state. The only change made here is giving
	// keys created before `expires` existed its default of 0 (no expiry).
	var state ApiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.Expires.IsNull() {
		state.Expires = types.Int64Value(0)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (r *ApiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every attribute forces replacement, so there is nothing to send to
	// Komodo. Update only runs to record `expires` for a key whose state
	// predates that attribute; the key and secret are carried over.
	var plan ApiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ApiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ApiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteApiKeyForServiceUser(ctx, state.Key.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting API key",
			fmt.Sprintf("failed deleting API key %q: %v", state.Name.ValueString(), err))
	}
}

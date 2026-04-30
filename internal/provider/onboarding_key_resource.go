package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

var (
	_ resource.Resource                = &OnboardingKeyResource{}
	_ resource.ResourceWithImportState = &OnboardingKeyResource{}
)

type OnboardingKeyResource struct {
	client *client.Client
}

type OnboardingKeyResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	Expires       types.Int64  `tfsdk:"expires"`
	Privileged    types.Bool   `tfsdk:"privileged"`
	CopyServer    types.String `tfsdk:"copy_server"`
	CreateBuilder types.Bool   `tfsdk:"create_builder"`
	Tags          types.List   `tfsdk:"tags"`
	PublicKey     types.String `tfsdk:"public_key"`
	PrivateKey    types.String `tfsdk:"private_key"`
}

func NewOnboardingKeyResource() resource.Resource {
	return &OnboardingKeyResource{}
}

func (r *OnboardingKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_onboarding_key"
}

func (r *OnboardingKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo server onboarding key. Periphery presents the matching private key on first contact and Komodo Core registers a Server using this key's settings. The private_key value is only available at creation time.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier (the public_key, which mutations key off).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Display name for the onboarding key.",
				Required:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the key can be used. Disable to revoke without deleting.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"expires": schema.Int64Attribute{
				Description: "Unix timestamp in milliseconds when the key expires. 0 = never.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"privileged": schema.BoolAttribute{
				Description: "Allow this key to enable disabled servers, remove address config, and update existing server public keys.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"copy_server": schema.StringAttribute{
				Description: "Server (ID or name) whose config to duplicate onto servers onboarded with this key.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"create_builder": schema.BoolAttribute{
				Description: "Whether to provision a Builder alongside each onboarded server.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"tags": schema.ListAttribute{
				Description: "Tags applied by default to servers onboarded with this key.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"public_key": schema.StringAttribute{
				Description: "SPKI base64 public key. Stable identifier used for Update / Delete.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"private_key": schema.StringAttribute{
				Description: "PKCS8 private key. Returned only at creation time and cannot be recovered later. Sensitive.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OnboardingKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OnboardingKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OnboardingKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := client.CreateOnboardingKeyParams{
		Name:          plan.Name.ValueString(),
		Expires:       plan.Expires.ValueInt64(),
		Privileged:    plan.Privileged.ValueBool(),
		CopyServer:    plan.CopyServer.ValueString(),
		CreateBuilder: plan.CreateBuilder.ValueBool(),
	}
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string
		plan.Tags.ElementsAs(ctx, &tags, false)
		params.Tags = tags
	}

	// Capture user intent BEFORE mapping the API response, since the API
	// always returns enabled=true on Create regardless of what we want.
	wantEnabled := plan.Enabled.IsNull() || plan.Enabled.IsUnknown() || plan.Enabled.ValueBool()

	created, err := r.client.CreateOnboardingKey(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Error creating onboarding key", err.Error())
		return
	}

	mapOnboardingKeyToModel(&created.Created, &plan)
	plan.PrivateKey = types.StringValue(created.PrivateKey)

	if !wantEnabled {
		enabled := false
		updated, err := r.client.UpdateOnboardingKey(ctx, client.UpdateOnboardingKeyParams{
			PublicKey: created.Created.PublicKey,
			Enabled:   &enabled,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error disabling onboarding key after create", err.Error())
			return
		}
		priv := plan.PrivateKey
		mapOnboardingKeyToModel(updated, &plan)
		plan.PrivateKey = priv
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OnboardingKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OnboardingKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.FindOnboardingKey(ctx, state.PublicKey.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading onboarding key", err.Error())
		return
	}

	// Preserve private_key in state — the API never returns it on read.
	prior := state.PrivateKey
	mapOnboardingKeyToModel(key, &state)
	state.PrivateKey = prior
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OnboardingKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OnboardingKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state OnboardingKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	enabled := plan.Enabled.ValueBool()
	expires := plan.Expires.ValueInt64()
	privileged := plan.Privileged.ValueBool()
	copyServer := plan.CopyServer.ValueString()
	createBuilder := plan.CreateBuilder.ValueBool()
	params := client.UpdateOnboardingKeyParams{
		PublicKey:     state.PublicKey.ValueString(),
		Name:          &name,
		Enabled:       &enabled,
		Expires:       &expires,
		Privileged:    &privileged,
		CopyServer:    &copyServer,
		CreateBuilder: &createBuilder,
	}
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string
		plan.Tags.ElementsAs(ctx, &tags, false)
		params.Tags = &tags
	}

	updated, err := r.client.UpdateOnboardingKey(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Error updating onboarding key", err.Error())
		return
	}

	mapOnboardingKeyToModel(updated, &plan)
	plan.PrivateKey = state.PrivateKey
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OnboardingKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OnboardingKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteOnboardingKey(ctx, state.PublicKey.ValueString()); err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError("Error deleting onboarding key", err.Error())
	}
}

func (r *OnboardingKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import takes the public_key. Mirror it onto both id and public_key.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("public_key"), req.ID)...)
}

func mapOnboardingKeyToModel(k *client.OnboardingKey, m *OnboardingKeyResourceModel) {
	m.ID = types.StringValue(k.PublicKey)
	m.PublicKey = types.StringValue(k.PublicKey)
	m.Name = types.StringValue(k.Name)
	m.Enabled = types.BoolValue(k.Enabled)
	m.Expires = types.Int64Value(k.Expires)
	m.Privileged = types.BoolValue(k.Privileged)
	m.CopyServer = types.StringValue(k.CopyServer)
	m.CreateBuilder = types.BoolValue(k.CreateBuilder)

	if len(k.Tags) > 0 {
		list, _ := types.ListValueFrom(context.Background(), types.StringType, k.Tags)
		m.Tags = list
	} else {
		m.Tags = types.ListNull(types.StringType)
	}
}

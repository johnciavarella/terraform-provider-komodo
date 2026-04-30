package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

var (
	_ resource.Resource                = &DockerRegistryAccountResource{}
	_ resource.ResourceWithImportState = &DockerRegistryAccountResource{}
)

type DockerRegistryAccountResource struct {
	client *client.Client
}

type DockerRegistryAccountResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Domain   types.String `tfsdk:"domain"`
	Username types.String `tfsdk:"username"`
	Token    types.String `tfsdk:"token"`
}

func NewDockerRegistryAccountResource() resource.Resource {
	return &DockerRegistryAccountResource{}
}

func (r *DockerRegistryAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_docker_registry_account"
}

func (r *DockerRegistryAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo docker registry account (credentials for accessing private image registries).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier of the docker registry account.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"domain": schema.StringAttribute{
				Description: "Registry domain without protocol (e.g. 'registry.hub.docker.com', 'ghcr.io'). Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"username": schema.StringAttribute{
				Description: "Registry account username. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"token": schema.StringAttribute{
				Description: "Personal access token or password for the registry account. Sensitive.",
				Required:    true,
				Sensitive:   true,
			},
		},
	}
}

func (r *DockerRegistryAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func mapDockerRegistryAccountToModel(d *client.DockerRegistryAccount, m *DockerRegistryAccountResourceModel) {
	m.ID = types.StringValue(d.ID)
	m.Domain = types.StringValue(d.Domain)
	m.Username = types.StringValue(d.Username)
	// Token is write-only — do not override from API (it is not returned)
}

func (r *DockerRegistryAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DockerRegistryAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.CreateDockerRegistryAccount(ctx,
		plan.Domain.ValueString(),
		plan.Username.ValueString(),
		plan.Token.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating docker registry account", err.Error())
		return
	}

	mapDockerRegistryAccountToModel(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DockerRegistryAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DockerRegistryAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetDockerRegistryAccount(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading docker registry account", err.Error())
		return
	}

	mapDockerRegistryAccountToModel(d, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DockerRegistryAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DockerRegistryAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state DockerRegistryAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.UpdateDockerRegistryAccount(ctx,
		state.ID.ValueString(),
		plan.Domain.ValueString(),
		plan.Username.ValueString(),
		plan.Token.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error updating docker registry account", err.Error())
		return
	}

	mapDockerRegistryAccountToModel(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DockerRegistryAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DockerRegistryAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteDockerRegistryAccount(ctx, state.ID.ValueString()); err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError("Error deleting docker registry account",
			fmt.Sprintf("failed deleting docker registry account %q/%q: %v", state.Domain.ValueString(), state.Username.ValueString(), err))
	}
}

func (r *DockerRegistryAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

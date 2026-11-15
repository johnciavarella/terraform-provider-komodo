package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                = &DeploymentResource{}
	_ resource.ResourceWithImportState = &DeploymentResource{}
	_ resource.ResourceWithModifyPlan  = &DeploymentResource{}
)

type DeploymentResource struct {
	client *client.Client
}

type DeploymentResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	ServerID             types.String `tfsdk:"server_id"`
	SwarmID              types.String `tfsdk:"swarm_id"`
	ImageType            types.String `tfsdk:"image_type"`
	Image                types.String `tfsdk:"image"`
	ImageRegistryAccount types.String `tfsdk:"image_registry_account"`
	SkipSecretInterp     types.Bool   `tfsdk:"skip_secret_interp"`
	PollForUpdates       types.Bool   `tfsdk:"poll_for_updates"`
	Links                types.List   `tfsdk:"links"`
	Network              types.String `tfsdk:"network"`
	RestartMode          types.String `tfsdk:"restart_mode"`
	Replicas             types.Int64  `tfsdk:"replicas"`
	TerminationSignal    types.String `tfsdk:"termination_signal"`
	TerminationTimeout   types.Int64  `tfsdk:"termination_timeout"`
	TermSignalLabels     types.String `tfsdk:"term_signal_labels"`
	Command              types.String `tfsdk:"command"`
	SendAlerts           types.Bool   `tfsdk:"send_alerts"`
	AutoUpdate           types.Bool   `tfsdk:"auto_update"`
	RedeployOnBuild      types.Bool   `tfsdk:"redeploy_on_build"`
	Ports                types.String `tfsdk:"ports"`
	Volumes              types.String `tfsdk:"volumes"`
	Environment          types.String `tfsdk:"environment"`
	Labels               types.String `tfsdk:"labels"`
	ExtraArgs            types.List   `tfsdk:"extra_args"`
	Started              types.Bool   `tfsdk:"started"`
	Deployed             types.Bool   `tfsdk:"deployed"`
	Tags                 types.List   `tfsdk:"tags"`
}

func NewDeploymentResource() resource.Resource {
	return &DeploymentResource{}
}

func (r *DeploymentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (r *DeploymentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo deployment (a container deployment).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier of the deployment.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Display name of the deployment.",
				Required:    true,
			},
			"server_id": schema.StringAttribute{
				Description: "The server to deploy on (ID or name).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"swarm_id": schema.StringAttribute{
				Description: "Swarm to attach the deployment to (ID or name).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"image_type": schema.StringAttribute{
				Description: `Image source type: "Image" for an external docker image, "Build" for a Komodo Build.`,
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("Image"),
			},
			"image": schema.StringAttribute{
				Description: `Docker image name (e.g. "nginx:latest") when image_type = "Image", or the build ID when image_type = "Build".`,
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"image_registry_account": schema.StringAttribute{
				Description: "Docker registry account (username) used to authenticate image pulls when image_type = \"Image\".",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"skip_secret_interp": schema.BoolAttribute{
				Description: "Skip Komodo variable / secret interpolation in environment.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"poll_for_updates": schema.BoolAttribute{
				Description: "Poll the configured registry for newer image digests.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"links": schema.ListAttribute{
				Description: "Quick links displayed in the Komodo UI for this deployment.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"network": schema.StringAttribute{
				Description:   "Docker network to connect to.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"restart_mode": schema.StringAttribute{
				Description:   "Restart policy (e.g., unless-stopped, always, on-failure, no).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"replicas": schema.Int64Attribute{
				Description: "Number of replicas for swarm-mode deployments. Ignored for plain Docker.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"termination_signal": schema.StringAttribute{
				Description: "Signal sent to the container on stop (e.g. SIGTERM, SIGINT). Defaults to Docker's default if empty.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"termination_timeout": schema.Int64Attribute{
				Description: "Seconds to wait for graceful shutdown before SIGKILL.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"term_signal_labels": schema.StringAttribute{
				Description: "Newline-separated SIGNAL=label pairs for per-stage termination signals.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"command": schema.StringAttribute{
				Description: "Override the container command.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"send_alerts": schema.BoolAttribute{
				Description: "Whether to send deployment state change alerts.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"auto_update": schema.BoolAttribute{
				Description: "Automatically redeploy when newer images are found.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"redeploy_on_build": schema.BoolAttribute{
				Description: "Redeploy the deployment whenever its attached Komodo build finishes. Only applies when image_type is \"Build\".",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"ports": schema.StringAttribute{
				Description: "Port mappings, newline-separated (e.g., \"8080:80\\n9090:90\").",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"volumes": schema.StringAttribute{
				Description: "Volume mounts, newline-separated (e.g., \"/host/path:/container/path\").",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"environment": schema.StringAttribute{
				Description: "Environment variables, newline-separated KEY=VALUE pairs.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"labels": schema.StringAttribute{
				Description: "Container labels, newline-separated KEY=VALUE pairs.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"extra_args": schema.ListAttribute{
				Description: "Extra arguments for docker run.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"started": schema.BoolAttribute{
				Description: "Whether the deployment container should be running. When true, runs Deploy (docker run). When false, runs DestroyDeployment (docker stop + rm). Intent-based and async.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"deployed": schema.BoolAttribute{
				Description: "When true, triggers a full Deploy (pull image + docker run) after create/update. Intent-based and async.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"tags": schema.ListAttribute{
				Description: "List of tag names to apply to this deployment.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *DeploymentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func deploymentConfigFromModel(ctx context.Context, m *DeploymentResourceModel) client.DeploymentConfig {
	imgType := m.ImageType.ValueString()
	if imgType == "" {
		imgType = "Image"
	}
	var imgParams client.DeploymentImageParams
	if imgType == "Build" {
		imgParams.BuildID = m.Image.ValueString()
	} else {
		imgParams.Image = m.Image.ValueString()
	}
	cfg := client.DeploymentConfig{
		ServerID:             m.ServerID.ValueString(),
		SwarmID:              m.SwarmID.ValueString(),
		Image:                client.DeploymentImage{Type: imgType, Params: imgParams},
		ImageRegistryAccount: m.ImageRegistryAccount.ValueString(),
		SkipSecretInterp:     client.BoolPtr(m.SkipSecretInterp.ValueBool()),
		PollForUpdates:       client.BoolPtr(m.PollForUpdates.ValueBool()),
		Network:              m.Network.ValueString(),
		RestartMode:          m.RestartMode.ValueString(),
		TerminationSignal:    m.TerminationSignal.ValueString(),
		TermSignalLabels:     m.TermSignalLabels.ValueString(),
		Command:              m.Command.ValueString(),
		Ports:                m.Ports.ValueString(),
		Volumes:              m.Volumes.ValueString(),
		Environment:          m.Environment.ValueString(),
		Labels:               m.Labels.ValueString(),
		SendAlerts:           client.BoolPtr(m.SendAlerts.ValueBool()),
		AutoUpdate:           client.BoolPtr(m.AutoUpdate.ValueBool()),
		Redeploy:             client.BoolPtr(m.RedeployOnBuild.ValueBool()),
	}
	if !m.Replicas.IsNull() && !m.Replicas.IsUnknown() {
		cfg.Replicas = client.IntPtr(int(m.Replicas.ValueInt64()))
	}
	if !m.TerminationTimeout.IsNull() && !m.TerminationTimeout.IsUnknown() {
		cfg.TerminationTimeout = client.IntPtr(int(m.TerminationTimeout.ValueInt64()))
	}
	if !m.ExtraArgs.IsNull() {
		var args []string
		m.ExtraArgs.ElementsAs(ctx, &args, false)
		cfg.ExtraArgs = args
	}
	if !m.Links.IsNull() && !m.Links.IsUnknown() {
		var v []string
		m.Links.ElementsAs(ctx, &v, false)
		cfg.Links = v
	}
	return cfg
}

func mapDeploymentToModel(ctx context.Context, c *client.Client, d *client.Deployment, m *DeploymentResourceModel) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.ServerID = types.StringValue(preserveServerIDName(ctx, c, m.ServerID.ValueString(), d.Config.ServerID))
	m.SwarmID = types.StringValue(d.Config.SwarmID)
	m.ImageType = types.StringValue(d.Config.Image.Type)
	if d.Config.Image.Type == "Build" {
		m.Image = types.StringValue(d.Config.Image.Params.BuildID)
	} else {
		m.Image = types.StringValue(d.Config.Image.Params.Image)
	}
	m.ImageRegistryAccount = types.StringValue(d.Config.ImageRegistryAccount)
	m.Network = types.StringValue(d.Config.Network)
	m.RestartMode = types.StringValue(d.Config.RestartMode)
	m.TerminationSignal = types.StringValue(d.Config.TerminationSignal)
	m.TermSignalLabels = types.StringValue(d.Config.TermSignalLabels)
	m.Command = types.StringValue(d.Config.Command)

	if d.Config.SendAlerts != nil {
		m.SendAlerts = types.BoolValue(*d.Config.SendAlerts)
	}
	if d.Config.AutoUpdate != nil {
		m.AutoUpdate = types.BoolValue(*d.Config.AutoUpdate)
	}
	if d.Config.Redeploy != nil {
		m.RedeployOnBuild = types.BoolValue(*d.Config.Redeploy)
	} else {
		m.RedeployOnBuild = types.BoolValue(false)
	}
	if d.Config.SkipSecretInterp != nil {
		m.SkipSecretInterp = types.BoolValue(*d.Config.SkipSecretInterp)
	}
	if d.Config.PollForUpdates != nil {
		m.PollForUpdates = types.BoolValue(*d.Config.PollForUpdates)
	}
	if d.Config.Replicas != nil {
		m.Replicas = types.Int64Value(int64(*d.Config.Replicas))
	}
	if d.Config.TerminationTimeout != nil {
		m.TerminationTimeout = types.Int64Value(int64(*d.Config.TerminationTimeout))
	}

	// Komodo appends a trailing newline when storing these multi-line strings.
	// Trim so plan/apply round-trips don't show fake drift.
	m.Ports = types.StringValue(strings.TrimRight(d.Config.Ports, "\n"))
	m.Volumes = types.StringValue(strings.TrimRight(d.Config.Volumes, "\n"))
	m.Environment = types.StringValue(strings.TrimRight(d.Config.Environment, "\n"))
	m.Labels = types.StringValue(strings.TrimRight(d.Config.Labels, "\n"))

	m.Links = mapListToModel(d.Config.Links, m.Links)

	if len(d.Config.ExtraArgs) > 0 {
		list, _ := types.ListValueFrom(ctx, types.StringType, d.Config.ExtraArgs)
		m.ExtraArgs = list
	} else if !m.ExtraArgs.IsNull() {
		m.ExtraArgs = types.ListNull(types.StringType)
	}

	m.Tags = mapTagsToModel(ctx, c, d.Tags, m.Tags)
}

func applyDeploymentStartedState(ctx context.Context, c *client.Client, id string, m *DeploymentResourceModel, diags *diag.Diagnostics) {
	if m.Started.ValueBool() {
		if err := c.DeployDeployment(ctx, id); err != nil {
			diags.AddError("Error starting deployment", fmt.Sprintf("failed to deploy %q: %v", id, err))
		}
	} else {
		if err := c.DestroyDeployment(ctx, id); err != nil {
			diags.AddError("Error stopping deployment", fmt.Sprintf("failed to destroy %q: %v", id, err))
		}
	}
}

func applyDeploymentDeployedState(ctx context.Context, c *client.Client, id string, m *DeploymentResourceModel, diags *diag.Diagnostics) {
	if m.Deployed.ValueBool() {
		time.Sleep(5 * time.Second)
		if err := c.DeployDeployment(ctx, id); err != nil {
			diags.AddError("Error deploying deployment", fmt.Sprintf("failed to deploy %q: %v", id, err))
		}
	}
}

func (r *DeploymentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DeploymentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := deploymentConfigFromModel(ctx, &plan)
	cfg.ServerID = resolveServerID(ctx, r.client, plan.ServerID.ValueString())

	deployment, err := r.client.CreateDeployment(ctx, client.CreateDeploymentParams{
		Name: plan.Name.ValueString(), Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating deployment", err.Error())
		return
	}

	tagsFromPlan := plan.Tags
	mapDeploymentToModel(ctx, r.client, deployment, &plan)
	applyResourceTags(ctx, r.client, "Deployment", deployment.ID, tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}

	applyDeploymentDeployedState(ctx, r.client, deployment.ID, &plan, &resp.Diagnostics)
	if !plan.Deployed.ValueBool() {
		applyDeploymentStartedState(ctx, r.client, deployment.ID, &plan, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DeploymentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DeploymentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deployment, err := r.client.GetDeployment(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading deployment", err.Error())
		return
	}

	mapDeploymentToModel(ctx, r.client, deployment, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DeploymentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DeploymentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state DeploymentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Name.ValueString() != state.Name.ValueString() {
		err := r.client.Write(ctx, "RenameDeployment", map[string]string{
			"id": state.ID.ValueString(), "name": plan.Name.ValueString(),
		}, nil)
		if err != nil {
			resp.Diagnostics.AddError("Error renaming deployment", err.Error())
			return
		}
	}

	cfg := deploymentConfigFromModel(ctx, &plan)
	cfg.ServerID = resolveServerID(ctx, r.client, plan.ServerID.ValueString())

	deployment, err := r.client.UpdateDeployment(ctx, client.UpdateDeploymentParams{
		ID: state.ID.ValueString(), Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating deployment", err.Error())
		return
	}

	tagsFromPlan := plan.Tags
	mapDeploymentToModel(ctx, r.client, deployment, &plan)
	applyResourceTags(ctx, r.client, "Deployment", state.ID.ValueString(), tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}

	applyDeploymentDeployedState(ctx, r.client, state.ID.ValueString(), &plan, &resp.Diagnostics)
	if !plan.Deployed.ValueBool() {
		applyDeploymentStartedState(ctx, r.client, state.ID.ValueString(), &plan, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DeploymentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DeploymentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDeployment(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError("Error deleting deployment", err.Error())
	}
}

func (r *DeploymentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *DeploymentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}
	var plan DeploymentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// server_id drift handled by Create/Update + preserveServerIDName.
	// Normalize empty extra_args list to null so it round-trips with Read
	// (Komodo treats null and [] identically on the wire, so we collapse
	// them to a single canonical form here to avoid perpetual diff).
	if !plan.ExtraArgs.IsNull() && !plan.ExtraArgs.IsUnknown() && len(plan.ExtraArgs.Elements()) == 0 {
		plan.ExtraArgs = types.ListNull(types.StringType)
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	}
}

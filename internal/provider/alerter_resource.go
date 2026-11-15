package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

var (
	_ resource.Resource                   = &AlerterResource{}
	_ resource.ResourceWithImportState    = &AlerterResource{}
	_ resource.ResourceWithValidateConfig = &AlerterResource{}
)

// Values Komodo accepts, from komodo_client 2.3.x.
var (
	alerterEndpointTypes = []string{"Custom", "Slack", "Discord", "Ntfy", "Pushover"}
	alerterAlertTypes    = []string{
		"Test", "SwarmUnhealthy", "ServerUnreachable", "ServerCpu", "ServerMem", "ServerDisk",
		"ServerVersionMismatch", "ContainerStateChange", "DeploymentImageUpdateAvailable",
		"DeploymentAutoUpdated", "StackStateChange", "StackImageUpdateAvailable", "StackAutoUpdated",
		"AwsBuilderTerminationFailed", "ResourceSyncPendingUpdates", "BuildFailed", "RepoBuildFailed",
		"ProcedureFailed", "ActionFailed", "ScheduleRun", "Custom",
	}
	resourceTargetTypes = []string{
		"Swarm", "Server", "Stack", "Deployment", "Build", "Repo",
		"Procedure", "Action", "Builder", "Alerter", "ResourceSync",
	}
)

type AlerterResource struct {
	client *client.Client
}

type alerterEndpointModel struct {
	Type  types.String `tfsdk:"type"`
	URL   types.String `tfsdk:"url"`
	Email types.String `tfsdk:"email"`
}

type resourceTargetModel struct {
	Type types.String `tfsdk:"type"`
	ID   types.String `tfsdk:"id"`
}

type AlerterResourceModel struct {
	ID              types.String          `tfsdk:"id"`
	Name            types.String          `tfsdk:"name"`
	Enabled         types.Bool            `tfsdk:"enabled"`
	Endpoint        *alerterEndpointModel `tfsdk:"endpoint"`
	AlertTypes      []types.String        `tfsdk:"alert_types"`
	Resources       []resourceTargetModel `tfsdk:"resources"`
	ExceptResources []resourceTargetModel `tfsdk:"except_resources"`
	Tags            types.List            `tfsdk:"tags"`
}

func NewAlerterResource() resource.Resource {
	return &AlerterResource{}
}

func (r *AlerterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alerter"
}

func resourceTargetListAttribute(description string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: description,
		Optional:    true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{Required: true, Description: "Resource type. One of: " + strings.Join(resourceTargetTypes, ", ") + "."},
				"id":   schema.StringAttribute{Required: true, Description: "Komodo ID of the resource (for example komodo_stack.app.id)."},
			},
		},
	}
}

func (r *AlerterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo alerter, which routes alerts (server unreachable, container state changes, failed builds, ...) to an endpoint such as Slack, Discord, ntfy, Pushover or a custom HTTP receiver. Maintenance windows are not managed by this resource and are left untouched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Unique identifier.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Display name."},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Whether the alerter sends alerts. Defaults to true (Komodo's own default for a new alerter is false).",
			},
			"endpoint": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Where alerts are sent.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{Required: true, Description: "Endpoint type. One of: " + strings.Join(alerterEndpointTypes, ", ") + ". Custom POSTs the alert as JSON to the URL."},
					"url":  schema.StringAttribute{Required: true, Sensitive: true, Description: "Endpoint URL: the Slack or Discord webhook URL, the ntfy topic URL, the Pushover messages URL including its token and user parameters, or the HTTP endpoint for Custom."},
					"email": schema.StringAttribute{
						Optional:    true,
						Description: "Ntfy only: e-mail address for ntfy e-mail notifications. SMTP must be configured on the ntfy server.",
					},
				},
			},
			"alert_types": schema.ListAttribute{
				Optional: true, ElementType: types.StringType,
				Description: "Only send these alert types. Empty or unset sends all types. Valid values: " + strings.Join(alerterAlertTypes, ", ") + ".",
			},
			"resources":        resourceTargetListAttribute("Only send alerts about these resources. Empty or unset sends alerts for all resources."),
			"except_resources": resourceTargetListAttribute("Never send alerts about these resources."),
			"tags":             schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: "List of tag names to apply to this alerter."},
		},
	}
}

func (r *AlerterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ValidateConfig checks enum values at plan time, where Komodo would
// otherwise reject them with a deserialization error during apply.
func (r *AlerterResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	// Each attribute is read on its own and skipped while unknown, so values
	// that are only known after apply do not fail validation.
	known := func(v types.String) bool { return !v.IsNull() && !v.IsUnknown() }

	var endpoint types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("endpoint"), &endpoint)...)
	if !endpoint.IsNull() && !endpoint.IsUnknown() {
		var ep alerterEndpointModel
		resp.Diagnostics.Append(endpoint.As(ctx, &ep, basetypes.ObjectAsOptions{})...)
		if known(ep.Type) {
			t := ep.Type.ValueString()
			if !slices.Contains(alerterEndpointTypes, t) {
				resp.Diagnostics.AddAttributeError(path.Root("endpoint").AtName("type"), "Invalid endpoint type",
					fmt.Sprintf("%q is not one of: %s.", t, strings.Join(alerterEndpointTypes, ", ")))
			} else if t != "Ntfy" && known(ep.Email) && ep.Email.ValueString() != "" {
				resp.Diagnostics.AddAttributeError(path.Root("endpoint").AtName("email"), "email is only supported for Ntfy",
					fmt.Sprintf("The %s endpoint has no email setting.", t))
			}
		}
	}

	var alertTypes types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("alert_types"), &alertTypes)...)
	if !alertTypes.IsNull() && !alertTypes.IsUnknown() {
		var vals []types.String
		resp.Diagnostics.Append(alertTypes.ElementsAs(ctx, &vals, false)...)
		for i, at := range vals {
			if known(at) && !slices.Contains(alerterAlertTypes, at.ValueString()) {
				resp.Diagnostics.AddAttributeError(path.Root("alert_types").AtListIndex(i), "Invalid alert type",
					fmt.Sprintf("%q is not one of: %s.", at.ValueString(), strings.Join(alerterAlertTypes, ", ")))
			}
		}
	}

	for _, attr := range []string{"resources", "except_resources"} {
		var list types.List
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(attr), &list)...)
		if list.IsNull() || list.IsUnknown() {
			continue
		}
		var targets []resourceTargetModel
		resp.Diagnostics.Append(list.ElementsAs(ctx, &targets, false)...)
		for i, t := range targets {
			if known(t.Type) && !slices.Contains(resourceTargetTypes, t.Type.ValueString()) {
				resp.Diagnostics.AddAttributeError(path.Root(attr).AtListIndex(i).AtName("type"), "Invalid resource type",
					fmt.Sprintf("%q is not one of: %s.", t.Type.ValueString(), strings.Join(resourceTargetTypes, ", ")))
			}
		}
	}
}

func resourceTargetsFromModel(in []resourceTargetModel) []client.ResourceTarget {
	out := make([]client.ResourceTarget, 0, len(in))
	for _, t := range in {
		out = append(out, client.ResourceTarget{Type: t.Type.ValueString(), ID: t.ID.ValueString()})
	}
	return out
}

// resourceTargetsToModel keeps the configured shape when Komodo returns no
// targets: an unset attribute stays null and an explicit [] stays [].
func resourceTargetsToModel(in []client.ResourceTarget, current []resourceTargetModel) []resourceTargetModel {
	if len(in) == 0 {
		if current == nil {
			return nil
		}
		return []resourceTargetModel{}
	}
	out := make([]resourceTargetModel, 0, len(in))
	for _, t := range in {
		out = append(out, resourceTargetModel{Type: types.StringValue(t.Type), ID: types.StringValue(t.ID)})
	}
	return out
}

func alerterConfigFromModel(m *AlerterResourceModel) client.AlerterConfig {
	cfg := client.AlerterConfig{
		Enabled:         m.Enabled.ValueBool(),
		Resources:       resourceTargetsFromModel(m.Resources),
		ExceptResources: resourceTargetsFromModel(m.ExceptResources),
		AlertTypes:      make([]string, 0, len(m.AlertTypes)),
	}
	if m.Endpoint != nil {
		cfg.Endpoint = client.AlerterEndpoint{
			Type:  m.Endpoint.Type.ValueString(),
			URL:   m.Endpoint.URL.ValueString(),
			Email: m.Endpoint.Email.ValueString(),
		}
	}
	for _, at := range m.AlertTypes {
		cfg.AlertTypes = append(cfg.AlertTypes, at.ValueString())
	}
	return cfg
}

func mapAlerterToModel(ctx context.Context, c *client.Client, a *client.Alerter, m *AlerterResourceModel) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Enabled = types.BoolValue(a.Config.Enabled)

	email := types.StringNull()
	if a.Config.Endpoint.Email != "" {
		email = types.StringValue(a.Config.Endpoint.Email)
	} else if m.Endpoint != nil && !m.Endpoint.Email.IsNull() && !m.Endpoint.Email.IsUnknown() && m.Endpoint.Email.ValueString() == "" {
		email = types.StringValue("") // keep an explicit email = ""
	}
	m.Endpoint = &alerterEndpointModel{
		Type:  types.StringValue(a.Config.Endpoint.Type),
		URL:   types.StringValue(a.Config.Endpoint.URL),
		Email: email,
	}

	if len(a.Config.AlertTypes) == 0 {
		if m.AlertTypes != nil {
			m.AlertTypes = []types.String{}
		}
	} else {
		m.AlertTypes = make([]types.String, 0, len(a.Config.AlertTypes))
		for _, at := range a.Config.AlertTypes {
			m.AlertTypes = append(m.AlertTypes, types.StringValue(at))
		}
	}
	m.Resources = resourceTargetsToModel(a.Config.Resources, m.Resources)
	m.ExceptResources = resourceTargetsToModel(a.Config.ExceptResources, m.ExceptResources)
	m.Tags = mapTagsToModel(ctx, c, a.Tags, m.Tags)
}

func (r *AlerterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AlerterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	alerter, err := r.client.CreateAlerter(ctx, plan.Name.ValueString(), alerterConfigFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating alerter", err.Error())
		return
	}

	tagsFromPlan := plan.Tags
	mapAlerterToModel(ctx, r.client, alerter, &plan)
	applyResourceTags(ctx, r.client, "Alerter", alerter.ID, tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AlerterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AlerterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	alerter, err := r.client.GetAlerter(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading alerter", err.Error())
		return
	}

	mapAlerterToModel(ctx, r.client, alerter, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AlerterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AlerterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()

	if plan.Name.ValueString() != state.Name.ValueString() {
		if err := r.client.RenameAlerter(ctx, id, plan.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error renaming alerter", err.Error())
			return
		}
	}

	alerter, err := r.client.UpdateAlerter(ctx, id, alerterConfigFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating alerter", err.Error())
		return
	}

	tagsFromPlan := plan.Tags
	mapAlerterToModel(ctx, r.client, alerter, &plan)
	applyResourceTags(ctx, r.client, "Alerter", id, tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AlerterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AlerterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAlerter(ctx, state.ID.ValueString()); err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError("Error deleting alerter", err.Error())
	}
}

func (r *AlerterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

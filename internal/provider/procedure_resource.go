package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                   = &ProcedureResource{}
	_ resource.ResourceWithImportState    = &ProcedureResource{}
	_ resource.ResourceWithValidateConfig = &ProcedureResource{}
)

// procedureStagesPrivateKey holds the stages exactly as Komodo last returned
// them. Komodo rewrites execution params on save (it fills in defaults and
// replaces resource names with IDs), so the stored form differs from the
// configured one. Read compares the live stages with this copy: if they
// match, nothing changed out of band and the configured values are kept.
const procedureStagesPrivateKey = "komodo_stages"

type ProcedureResource struct {
	client *client.Client
}

type procedureExecutionModel struct {
	Type    types.String `tfsdk:"type"`
	Params  types.String `tfsdk:"params"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

type procedureStageModel struct {
	Name       types.String              `tfsdk:"name"`
	Enabled    types.Bool                `tfsdk:"enabled"`
	Executions []procedureExecutionModel `tfsdk:"executions"`
}

type ProcedureResourceModel struct {
	ID               types.String          `tfsdk:"id"`
	Name             types.String          `tfsdk:"name"`
	Stages           []procedureStageModel `tfsdk:"stages"`
	ScheduleFormat   types.String          `tfsdk:"schedule_format"`
	Schedule         types.String          `tfsdk:"schedule"`
	ScheduleEnabled  types.Bool            `tfsdk:"schedule_enabled"`
	ScheduleTimezone types.String          `tfsdk:"schedule_timezone"`
	ScheduleAlert    types.Bool            `tfsdk:"schedule_alert"`
	FailureAlert     types.Bool            `tfsdk:"failure_alert"`
	WebhookEnabled   types.Bool            `tfsdk:"webhook_enabled"`
	WebhookSecret    types.String          `tfsdk:"webhook_secret"`
	Tags             types.List            `tfsdk:"tags"`
	Description      types.String          `tfsdk:"description"`
	Template         types.Bool            `tfsdk:"template"`
}

func NewProcedureResource() resource.Resource {
	return &ProcedureResource{}
}

func (r *ProcedureResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_procedure"
}

func (r *ProcedureResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo procedure: an ordered list of stages, each running one or more executions (deploy a stack, run a build, pull a repo, ...) in parallel. A procedure can run on a schedule, from a webhook, or on demand.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Unique identifier.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Display name."},
			"stages": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Stages, run one after another. The executions inside a stage run in parallel.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":    schema.StringAttribute{Required: true, Description: "Stage name."},
						"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Whether the stage runs. A disabled stage is skipped."},
						"executions": schema.ListNestedAttribute{
							Required:    true,
							Description: "Executions run in parallel within this stage.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"type": schema.StringAttribute{Required: true, Description: "Execution type, for example DeployStack, BatchDeployStackIfChanged, Deploy, RunBuild, PullRepo, RunProcedure, RunAction, GlobalAutoUpdate, BackupCoreDatabase or Sleep. See the Komodo API reference for the full list."},
									"params": schema.StringAttribute{
										Optional: true, Computed: true, Default: stringdefault.StaticString("{}"),
										Description: "Parameters of the execution as a JSON object, for example jsonencode({ stack = komodo_stack.app.id }). Komodo fills in defaults and replaces resource names with IDs when it saves; those rewrites are not shown as changes.",
									},
									"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Whether this execution runs."},
								},
							},
						},
					},
				},
			},
			"schedule_format": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("English"),
				Description: "How schedule is written: \"English\" (for example \"every day at 3am\") or \"Cron\".",
			},
			"schedule": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString(""),
				Description: "When to run the procedure. Cron uses six fields with seconds first, for example \"0 0 3 * * ?\". Empty means no schedule.",
			},
			"schedule_enabled":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Whether the schedule is active when one is set."},
			"schedule_timezone": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "TZ identifier for the schedule, for example \"America/New_York\". Empty uses Komodo Core's timezone."},
			"schedule_alert":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Send an alert each time the schedule runs."},
			"failure_alert":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Send an alert when the procedure fails."},
			"webhook_enabled":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Whether incoming webhooks trigger the procedure."},
			"webhook_secret":    schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, Default: stringdefault.StaticString(""), Description: "Alternate webhook secret for this procedure. Empty uses the default secret from Komodo Core's config."},
			"description":       metaDescriptionAttribute(),
			"template":          metaTemplateAttribute(),
			"tags":              schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: "List of tag names to apply to this procedure."},
		},
	}
}

func (r *ProcedureResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ValidateConfig checks schedule_format and that every params value is a JSON
// object. Values not known until apply are skipped.
func (r *ProcedureResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	validateResourceName(ctx, req.Config, &resp.Diagnostics)

	var format types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("schedule_format"), &format)...)
	if !format.IsNull() && !format.IsUnknown() && format.ValueString() != "English" && format.ValueString() != "Cron" {
		resp.Diagnostics.AddAttributeError(path.Root("schedule_format"), "Invalid schedule format",
			fmt.Sprintf("%q is not one of: English, Cron.", format.ValueString()))
	}

	var stages types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("stages"), &stages)...)
	if stages.IsNull() || stages.IsUnknown() {
		return
	}
	for i, stageVal := range stages.Elements() {
		stageObj, ok := stageVal.(types.Object)
		if !ok || stageObj.IsNull() || stageObj.IsUnknown() {
			continue
		}
		execs, ok := stageObj.Attributes()["executions"].(types.List)
		if !ok || execs.IsNull() || execs.IsUnknown() {
			continue
		}
		for j, execVal := range execs.Elements() {
			execObj, ok := execVal.(types.Object)
			if !ok || execObj.IsNull() || execObj.IsUnknown() {
				continue
			}
			execPath := path.Root("stages").AtListIndex(i).AtName("executions").AtListIndex(j)
			if t, ok := execObj.Attributes()["type"].(types.String); ok && !t.IsNull() && !t.IsUnknown() && t.ValueString() == "" {
				resp.Diagnostics.AddAttributeError(execPath.AtName("type"), "Missing execution type", "type must not be empty.")
			}
			p, ok := execObj.Attributes()["params"].(types.String)
			if !ok || p.IsNull() || p.IsUnknown() {
				continue
			}
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(p.ValueString()), &obj); err != nil || obj == nil {
				resp.Diagnostics.AddAttributeError(execPath.AtName("params"), "Invalid execution params",
					"params must be a JSON object, for example jsonencode({ stack = \"my-stack\" }).")
			}
		}
	}
}

func procedureStagesFromModel(in []procedureStageModel) []client.ProcedureStage {
	out := make([]client.ProcedureStage, 0, len(in))
	for _, s := range in {
		stage := client.ProcedureStage{
			Name:       s.Name.ValueString(),
			Enabled:    s.Enabled.ValueBool(),
			Executions: make([]client.EnabledExecution, 0, len(s.Executions)),
		}
		for _, e := range s.Executions {
			params := e.Params.ValueString()
			if params == "" {
				params = "{}"
			}
			stage.Executions = append(stage.Executions, client.EnabledExecution{
				Execution: client.Execution{Type: e.Type.ValueString(), Params: json.RawMessage(params)},
				Enabled:   e.Enabled.ValueBool(),
			})
		}
		out = append(out, stage)
	}
	return out
}

// procedureStagesToModel renders stages as Komodo returned them. An empty
// result keeps the configured shape (null stays null, [] stays []).
func procedureStagesToModel(in []client.ProcedureStage, current []procedureStageModel) []procedureStageModel {
	if len(in) == 0 {
		if current == nil {
			return nil
		}
		return []procedureStageModel{}
	}
	out := make([]procedureStageModel, 0, len(in))
	for _, s := range in {
		stage := procedureStageModel{
			Name:       types.StringValue(s.Name),
			Enabled:    types.BoolValue(s.Enabled),
			Executions: make([]procedureExecutionModel, 0, len(s.Executions)),
		}
		for _, e := range s.Executions {
			params := "{}"
			if len(e.Execution.Params) > 0 {
				var compact bytes.Buffer
				if err := json.Compact(&compact, e.Execution.Params); err == nil {
					params = compact.String()
				}
			}
			stage.Executions = append(stage.Executions, procedureExecutionModel{
				Type:    types.StringValue(e.Execution.Type),
				Params:  types.StringValue(params),
				Enabled: types.BoolValue(e.Enabled),
			})
		}
		out = append(out, stage)
	}
	return out
}

func procedureConfigFromModel(m *ProcedureResourceModel) client.ProcedureConfig {
	return client.ProcedureConfig{
		Stages:           procedureStagesFromModel(m.Stages),
		ScheduleFormat:   m.ScheduleFormat.ValueString(),
		Schedule:         m.Schedule.ValueString(),
		ScheduleEnabled:  m.ScheduleEnabled.ValueBool(),
		ScheduleTimezone: m.ScheduleTimezone.ValueString(),
		ScheduleAlert:    m.ScheduleAlert.ValueBool(),
		FailureAlert:     m.FailureAlert.ValueBool(),
		WebhookEnabled:   m.WebhookEnabled.ValueBool(),
		WebhookSecret:    m.WebhookSecret.ValueString(),
	}
}

// mapProcedureToModel copies everything except stages, which each caller
// handles according to whether it trusts the plan or the API.
func mapProcedureToModel(ctx context.Context, c *client.Client, p *client.Procedure, m *ProcedureResourceModel) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	format := p.Config.ScheduleFormat
	if format == "" {
		format = "English"
	}
	m.ScheduleFormat = types.StringValue(format)
	m.Schedule = types.StringValue(p.Config.Schedule)
	m.ScheduleEnabled = types.BoolValue(p.Config.ScheduleEnabled)
	m.ScheduleTimezone = types.StringValue(p.Config.ScheduleTimezone)
	m.ScheduleAlert = types.BoolValue(p.Config.ScheduleAlert)
	m.FailureAlert = types.BoolValue(p.Config.FailureAlert)
	m.WebhookEnabled = types.BoolValue(p.Config.WebhookEnabled)
	m.WebhookSecret = types.StringValue(p.Config.WebhookSecret)
	m.Tags = mapTagsToModel(ctx, c, p.Tags, m.Tags)
}

type privateStateSetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

func saveProcedureStages(ctx context.Context, private privateStateSetter, stages []client.ProcedureStage, diags *diag.Diagnostics) {
	canonical, err := client.CanonicalStagesJSON(stages)
	if err != nil {
		diags.AddError("Error recording procedure stages", err.Error())
		return
	}
	diags.Append(private.SetKey(ctx, procedureStagesPrivateKey, canonical)...)
}

func (r *ProcedureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProcedureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	procedure, err := r.client.CreateProcedure(ctx, plan.Name.ValueString(), procedureConfigFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating procedure", err.Error())
		return
	}

	// Stages stay as planned; Komodo's rewritten copy goes to private state.
	tagsFromPlan := plan.Tags
	mapProcedureToModel(ctx, r.client, procedure, &plan)
	saveProcedureStages(ctx, resp.Private, procedure.Config.Stages, &resp.Diagnostics)
	applyResourceTags(ctx, r.client, "Procedure", procedure.ID, tagsFromPlan, &resp.Diagnostics)
	applyResourceMeta(ctx, r.client, "Procedure", procedure.ID, plan.Description, plan.Template, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProcedureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProcedureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	procedure, err := r.client.GetProcedure(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading procedure", err.Error())
		return
	}

	mapProcedureToModel(ctx, r.client, procedure, &state)

	live, err := client.CanonicalStagesJSON(procedure.Config.Stages)
	if err != nil {
		resp.Diagnostics.AddError("Error reading procedure stages", err.Error())
		return
	}
	recorded, diags := req.Private.GetKey(ctx, procedureStagesPrivateKey)
	resp.Diagnostics.Append(diags...)
	if recorded == nil || !bytes.Equal(recorded, live) {
		// Imported, or changed outside Terraform: show what Komodo has.
		state.Stages = procedureStagesToModel(procedure.Config.Stages, state.Stages)
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, procedureStagesPrivateKey, live)...)
	}
	readResourceMeta(ctx, r.client, "Procedure", state.ID.ValueString(), &state.Description, &state.Template, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProcedureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ProcedureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()

	if plan.Name.ValueString() != state.Name.ValueString() {
		if err := r.client.RenameProcedure(ctx, id, plan.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error renaming procedure", err.Error())
			return
		}
	}

	procedure, err := r.client.UpdateProcedure(ctx, id, procedureConfigFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating procedure", err.Error())
		return
	}

	tagsFromPlan := plan.Tags
	mapProcedureToModel(ctx, r.client, procedure, &plan)
	saveProcedureStages(ctx, resp.Private, procedure.Config.Stages, &resp.Diagnostics)
	applyResourceTags(ctx, r.client, "Procedure", id, tagsFromPlan, &resp.Diagnostics)
	applyResourceMeta(ctx, r.client, "Procedure", id, plan.Description, plan.Template, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProcedureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProcedureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteProcedure(ctx, state.ID.ValueString()); err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError("Error deleting procedure", err.Error())
	}
}

func (r *ProcedureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

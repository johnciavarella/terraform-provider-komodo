package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	_ resource.Resource                = &RepoResource{}
	_ resource.ResourceWithImportState = &RepoResource{}
	_ resource.ResourceWithModifyPlan  = &RepoResource{}
)

type RepoResource struct {
	client *client.Client
}

type RepoResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ServerID         types.String `tfsdk:"server_id"`
	BuilderID        types.String `tfsdk:"builder_id"`
	GitProvider      types.String `tfsdk:"git_provider"`
	GitHTTPS         types.Bool   `tfsdk:"git_https"`
	GitAccount       types.String `tfsdk:"git_account"`
	Repo             types.String `tfsdk:"repo"`
	Branch           types.String `tfsdk:"branch"`
	Commit           types.String `tfsdk:"commit"`
	Path             types.String `tfsdk:"path"`
	OnClonePath      types.String `tfsdk:"on_clone_path"`
	OnCloneCommand   types.String `tfsdk:"on_clone_command"`
	OnPullPath       types.String `tfsdk:"on_pull_path"`
	OnPullCommand    types.String `tfsdk:"on_pull_command"`
	WebhookEnabled   types.Bool   `tfsdk:"webhook_enabled"`
	WebhookSecret    types.String `tfsdk:"webhook_secret"`
	Links            types.List   `tfsdk:"links"`
	Environment      types.String `tfsdk:"environment"`
	EnvFilePath      types.String `tfsdk:"env_file_path"`
	SkipSecretInterp types.Bool   `tfsdk:"skip_secret_interp"`
	Tags             types.List   `tfsdk:"tags"`
}

func NewRepoResource() resource.Resource {
	return &RepoResource{}
}

func (r *RepoResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_repo"
}

func (r *RepoResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo repo (a git repository cloned on a server).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Unique identifier.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":               schema.StringAttribute{Required: true, Description: "Display name."},
			"server_id":          schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Server to clone onto (ID or name)."},
			"builder_id":         schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Builder to clone onto (ID or name). Set this OR server_id, not both."},
			"git_provider":       schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("github.com"), Description: "Git provider domain."},
			"git_https":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Use HTTPS to clone."},
			"git_account":        schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Git account for private repos."},
			"repo":               schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Git repository (namespace/repo_name)."},
			"branch":             schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Branch."},
			"commit":             schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Specific commit hash."},
			"path":               schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Path on the server where the repo is cloned. Defaults to the repo name."},
			"on_clone_path":      schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Working directory for the on_clone command."},
			"on_clone_command":   schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Command to run after cloning."},
			"on_pull_path":       schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Working directory for the on_pull command."},
			"on_pull_command":    schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Command to run after pulling."},
			"webhook_enabled":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Enable webhooks."},
			"webhook_secret":     schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, Default: stringdefault.StaticString(""), Description: "Alternate webhook secret for this repo."},
			"links":              schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: "Quick links displayed in the Komodo UI."},
			"environment":        schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Environment variables (KEY=VALUE, newline separated) for on_clone / on_pull commands."},
			"env_file_path":      schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(".env"), Description: "Filename to write environment to. Komodo defaults this to .env if empty."},
			"skip_secret_interp": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Skip Komodo variable / secret interpolation in environment."},
			"tags":               schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: "List of tag names to apply to this repo."},
		},
	}
}

func (r *RepoResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func repoConfigFromModel(ctx context.Context, m *RepoResourceModel) client.RepoConfig {
	cfg := client.RepoConfig{
		ServerID:         m.ServerID.ValueString(),
		BuilderID:        m.BuilderID.ValueString(),
		GitProvider:      m.GitProvider.ValueString(),
		GitHTTPS:         client.BoolPtr(m.GitHTTPS.ValueBool()),
		GitAccount:       m.GitAccount.ValueString(),
		Repo:             m.Repo.ValueString(),
		Branch:           m.Branch.ValueString(),
		Commit:           m.Commit.ValueString(),
		Path:             m.Path.ValueString(),
		WebhookEnabled:   client.BoolPtr(m.WebhookEnabled.ValueBool()),
		WebhookSecret:    m.WebhookSecret.ValueString(),
		Environment:      m.Environment.ValueString(),
		EnvFilePath:      m.EnvFilePath.ValueString(),
		SkipSecretInterp: client.BoolPtr(m.SkipSecretInterp.ValueBool()),
	}
	if m.OnClonePath.ValueString() != "" || m.OnCloneCommand.ValueString() != "" {
		cfg.OnClone = &client.SystemCommand{
			Path:    m.OnClonePath.ValueString(),
			Command: m.OnCloneCommand.ValueString(),
		}
	}
	if m.OnPullPath.ValueString() != "" || m.OnPullCommand.ValueString() != "" {
		cfg.OnPull = &client.SystemCommand{
			Path:    m.OnPullPath.ValueString(),
			Command: m.OnPullCommand.ValueString(),
		}
	}
	if !m.Links.IsNull() && !m.Links.IsUnknown() {
		var v []string
		m.Links.ElementsAs(ctx, &v, false)
		cfg.Links = v
	}
	return cfg
}

func mapRepoToModel(ctx context.Context, c *client.Client, r *client.Repo, m *RepoResourceModel) {
	m.ID = types.StringValue(r.ID)
	m.Name = types.StringValue(r.Name)
	m.ServerID = types.StringValue(preserveServerIDName(ctx, c, m.ServerID.ValueString(), r.Config.ServerID))
	m.BuilderID = types.StringValue(r.Config.BuilderID)
	m.GitProvider = types.StringValue(r.Config.GitProvider)
	m.GitAccount = types.StringValue(r.Config.GitAccount)
	m.Repo = types.StringValue(r.Config.Repo)
	m.Branch = types.StringValue(r.Config.Branch)
	m.Commit = types.StringValue(r.Config.Commit)
	m.Path = types.StringValue(r.Config.Path)
	m.WebhookSecret = types.StringValue(r.Config.WebhookSecret)
	m.Environment = types.StringValue(strings.TrimRight(r.Config.Environment, "\n"))
	m.EnvFilePath = types.StringValue(r.Config.EnvFilePath)

	if r.Config.OnClone != nil {
		m.OnClonePath = types.StringValue(strings.TrimRight(r.Config.OnClone.Path, "\n"))
		m.OnCloneCommand = types.StringValue(strings.TrimRight(r.Config.OnClone.Command, "\n"))
	} else {
		m.OnClonePath = types.StringValue("")
		m.OnCloneCommand = types.StringValue("")
	}
	if r.Config.OnPull != nil {
		m.OnPullPath = types.StringValue(strings.TrimRight(r.Config.OnPull.Path, "\n"))
		m.OnPullCommand = types.StringValue(strings.TrimRight(r.Config.OnPull.Command, "\n"))
	} else {
		m.OnPullPath = types.StringValue("")
		m.OnPullCommand = types.StringValue("")
	}

	m.Links = mapListToModel(r.Config.Links, m.Links)

	if r.Config.GitHTTPS != nil {
		m.GitHTTPS = types.BoolValue(*r.Config.GitHTTPS)
	}
	if r.Config.WebhookEnabled != nil {
		m.WebhookEnabled = types.BoolValue(*r.Config.WebhookEnabled)
	}
	if r.Config.SkipSecretInterp != nil {
		m.SkipSecretInterp = types.BoolValue(*r.Config.SkipSecretInterp)
	}

	m.Tags = mapTagsToModel(ctx, c, r.Tags, m.Tags)
}

func (r *RepoResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RepoResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := repoConfigFromModel(ctx, &plan)
	cfg.ServerID = resolveServerID(ctx, r.client, plan.ServerID.ValueString())

	repo, err := r.client.CreateRepo(ctx, client.CreateRepoParams{
		Name: plan.Name.ValueString(), Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating repo", err.Error())
		return
	}

	tagsFromPlan := plan.Tags
	mapRepoToModel(ctx, r.client, repo, &plan)
	applyResourceTags(ctx, r.client, "Repo", repo.ID, tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RepoResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RepoResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repo, err := r.client.GetRepo(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading repo", err.Error())
		return
	}

	mapRepoToModel(ctx, r.client, repo, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RepoResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RepoResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state RepoResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Name.ValueString() != state.Name.ValueString() {
		err := r.client.Write(ctx, "RenameRepo", map[string]string{
			"id": state.ID.ValueString(), "name": plan.Name.ValueString(),
		}, nil)
		if err != nil {
			resp.Diagnostics.AddError("Error renaming repo", err.Error())
			return
		}
	}

	cfg := repoConfigFromModel(ctx, &plan)
	cfg.ServerID = resolveServerID(ctx, r.client, plan.ServerID.ValueString())

	repo, err := r.client.UpdateRepo(ctx, client.UpdateRepoParams{
		ID: state.ID.ValueString(), Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating repo", err.Error())
		return
	}

	tagsFromPlan := plan.Tags
	mapRepoToModel(ctx, r.client, repo, &plan)
	applyResourceTags(ctx, r.client, "Repo", state.ID.ValueString(), tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RepoResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RepoResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteRepo(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError("Error deleting repo", err.Error())
	}
}

func (r *RepoResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *RepoResource) ModifyPlan(_ context.Context, _ resource.ModifyPlanRequest, _ *resource.ModifyPlanResponse) {
	// server_id name→ID drift is handled by Create/Update calling
	// resolveServerID for the API request and mapRepoToModel using
	// preserveServerIDName so state keeps the user's literal.
}

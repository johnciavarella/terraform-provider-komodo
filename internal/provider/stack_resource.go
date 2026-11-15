package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

// useStateForUnknownIncludingNull is like UseStateForUnknown but also uses
// the state value when it is null. The standard modifier skips null state,
// causing perpetual unknown diffs for optional computed lists the API returns
// as empty/null.
type useStateForUnknownIncludingNull struct{}

func (m useStateForUnknownIncludingNull) Description(_ context.Context) string {
	return "Uses the prior state value (including null) when the planned value is unknown."
}
func (m useStateForUnknownIncludingNull) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (m useStateForUnknownIncludingNull) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	// Only act when the planned value is unknown and the resource already exists.
	if !req.PlanValue.IsUnknown() || req.State.Raw.IsNull() {
		return
	}
	resp.PlanValue = req.StateValue
}

var (
	_ resource.Resource                = &StackResource{}
	_ resource.ResourceWithImportState = &StackResource{}
	_ resource.ResourceWithModifyPlan  = &StackResource{}
)

type StackResource struct {
	client *client.Client
}

type StackResourceModel struct {
	ID                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	ServerID                 types.String `tfsdk:"server_id"`
	SwarmID                  types.String `tfsdk:"swarm_id"`
	Links                    types.List   `tfsdk:"links"`
	ProjectName              types.String `tfsdk:"project_name"`
	AutoPull                 types.Bool   `tfsdk:"auto_pull"`
	RunBuild                 types.Bool   `tfsdk:"run_build"`
	PollForUpdates           types.Bool   `tfsdk:"poll_for_updates"`
	AutoUpdate               types.Bool   `tfsdk:"auto_update"`
	AutoUpdateAllServices    types.Bool   `tfsdk:"auto_update_all_services"`
	DestroyBeforeDeploy      types.Bool   `tfsdk:"destroy_before_deploy"`
	SkipSecretInterp         types.Bool   `tfsdk:"skip_secret_interp"`
	LinkedRepo               types.String `tfsdk:"linked_repo"`
	GitProvider              types.String `tfsdk:"git_provider"`
	GitHTTPS                 types.Bool   `tfsdk:"git_https"`
	GitAccount               types.String `tfsdk:"git_account"`
	RegistryProvider         types.String `tfsdk:"registry_provider"`
	RegistryAccount          types.String `tfsdk:"registry_account"`
	Repo                     types.String `tfsdk:"repo"`
	Branch                   types.String `tfsdk:"branch"`
	Commit                   types.String `tfsdk:"commit"`
	ClonePath                types.String `tfsdk:"clone_path"`
	Reclone                  types.Bool   `tfsdk:"reclone"`
	FilePaths                types.List   `tfsdk:"file_paths"`
	FilesOnHost              types.Bool   `tfsdk:"files_on_host"`
	FileContents             types.String `tfsdk:"file_contents"`
	Environment              types.String `tfsdk:"environment"`
	RunDirectory             types.String `tfsdk:"run_directory"`
	EnvFilePath              types.String `tfsdk:"env_file_path"`
	AdditionalEnvFiles       types.List   `tfsdk:"additional_env_files"`
	WebhookEnabled           types.Bool   `tfsdk:"webhook_enabled"`
	WebhookSecret            types.String `tfsdk:"webhook_secret"`
	WebhookForceDeploy       types.Bool   `tfsdk:"webhook_force_deploy"`
	PreDeployPath            types.String `tfsdk:"pre_deploy_path"`
	PreDeployCommand         types.String `tfsdk:"pre_deploy_command"`
	PostDeployPath           types.String `tfsdk:"post_deploy_path"`
	PostDeployCommand        types.String `tfsdk:"post_deploy_command"`
	ExtraArgs                types.List   `tfsdk:"extra_args"`
	BuildExtraArgs           types.List   `tfsdk:"build_extra_args"`
	ComposeCmdWrapper        types.String `tfsdk:"compose_cmd_wrapper"`
	ComposeCmdWrapperInclude types.List   `tfsdk:"compose_cmd_wrapper_include"`
	IgnoreServices           types.List   `tfsdk:"ignore_services"`
	AutoUpdateSkipServices   types.List   `tfsdk:"auto_update_skip_services"`
	ConfigFiles              types.List   `tfsdk:"config_files"`
	SendAlerts               types.Bool   `tfsdk:"send_alerts"`
	Started                  types.Bool   `tfsdk:"started"`
	Deployed                 types.Bool   `tfsdk:"deployed"`
	Tags                     types.List   `tfsdk:"tags"`
}

// additionalEnvFileObjectType is the Terraform object type for entries in
// `additional_env_files`. v2 expanded these from plain strings to {path, track}
// objects.
var additionalEnvFileObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"path":  types.StringType,
		"track": types.BoolType,
	},
}

// stackConfigFileObjectType is the Terraform object type for entries in
// `config_files`.
var stackConfigFileObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"path":     types.StringType,
		"services": types.ListType{ElemType: types.StringType},
		"requires": types.StringType,
	},
}

func NewStackResource() resource.Resource {
	return &StackResource{}
}

func (r *StackResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stack"
}

func (r *StackResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Komodo stack (Docker Compose stack).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier of the stack.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Display name of the stack.",
				Required:    true,
			},
			"server_id": schema.StringAttribute{
				Description: "The server to deploy the stack on (ID or name).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("Local"),
			},
			"swarm_id": schema.StringAttribute{
				Description: "Swarm to attach the stack to (ID or name). Mutually exclusive with server_id for swarm-mode deployments.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"links": schema.ListAttribute{
				Description: "Quick links displayed in the Komodo UI for this stack.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"project_name": schema.StringAttribute{
				Description: "Custom project name for docker compose -p. Defaults to the stack name.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"auto_pull": schema.BoolAttribute{
				Description: "Whether to automatically compose pull before redeploying.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"run_build": schema.BoolAttribute{
				Description: "Whether to docker compose build before deploy.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"poll_for_updates": schema.BoolAttribute{
				Description: "Poll connected docker registries for newer images.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"auto_update": schema.BoolAttribute{
				Description: "Whether to automatically redeploy when newer images are found.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"auto_update_all_services": schema.BoolAttribute{
				Description: "Whether auto-update redeploys every service (true) or only services with updated images (false).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"destroy_before_deploy": schema.BoolAttribute{
				Description: "Whether to run docker compose down before compose up.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"skip_secret_interp": schema.BoolAttribute{
				Description: "Skip Komodo variable / secret interpolation in compose files and environment.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"linked_repo": schema.StringAttribute{
				Description: "Komodo Repo resource (ID or name) to source compose files from. Alternative to inline git_provider/repo settings.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"git_provider": schema.StringAttribute{
				Description: "Git provider domain (e.g., github.com).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("github.com"),
			},
			"git_https": schema.BoolAttribute{
				Description: "Whether to use HTTPS to clone the repo.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"git_account": schema.StringAttribute{
				Description: "Git account for accessing private repos.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"registry_provider": schema.StringAttribute{
				Description: "Docker registry domain used for image pulls (e.g. 'ghcr.io', 'registry.hub.docker.com'). Must match the domain of the registry account.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"registry_account": schema.StringAttribute{
				Description: "Username of the docker registry account to authenticate with. Must match a komodo_docker_registry_account username for the given registry_provider.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"repo": schema.StringAttribute{
				Description: "Git repository in namespace/repo_name format.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"branch": schema.StringAttribute{
				Description: "Branch of the repo.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("main"),
			},
			"commit": schema.StringAttribute{
				Description: "Specific commit hash.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"clone_path": schema.StringAttribute{
				Description: "Custom path on the server / builder to clone the repo into.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"reclone": schema.BoolAttribute{
				Description: "Delete the existing clone and re-clone from scratch on every deploy.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"file_paths": schema.ListAttribute{
				Description: "Paths to docker-compose.yml files.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					useStateForUnknownIncludingNull{},
				},
			},
			"files_on_host": schema.BoolAttribute{
				Description: "Source files from the host instead of a git repo.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"file_contents": schema.StringAttribute{
				Description: "Inline compose file contents for UI-managed stacks (alternative to repo-based files).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"environment": schema.StringAttribute{
				Description: "Environment variables written to the env file before docker compose up (KEY=VALUE format, newline separated).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"run_directory": schema.StringAttribute{
				Description: "Directory to cd into before running docker compose.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"env_file_path": schema.StringAttribute{
				Description: "Name of the environment file written before docker compose up.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(".env"),
			},
			"additional_env_files": schema.ListNestedAttribute{
				Description: "Additional env files to attach with --env-file. Each entry is an object with `path` (relative to run_directory) and `track` (whether Komodo reads/diffs the file's contents).",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path":  schema.StringAttribute{Required: true, Description: "Path of the env file relative to run_directory."},
						"track": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Whether Komodo reads/displays/validates the file. If false, the file is only passed via --env-file."},
					},
				},
				PlanModifiers: []planmodifier.List{
					useStateForUnknownIncludingNull{},
				},
			},
			"webhook_enabled": schema.BoolAttribute{
				Description: "Whether incoming webhooks trigger action.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"webhook_secret": schema.StringAttribute{
				Description: "Alternate webhook secret for this stack.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Default:     stringdefault.StaticString(""),
			},
			"webhook_force_deploy": schema.BoolAttribute{
				Description: "Always execute deployment without comparing changes when webhook fires.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"pre_deploy_path": schema.StringAttribute{
				Description: "Working directory for the pre-deploy command.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"pre_deploy_command": schema.StringAttribute{
				Description: "Command to run before the stack is deployed.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"post_deploy_path": schema.StringAttribute{
				Description: "Working directory for the post-deploy command.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"post_deploy_command": schema.StringAttribute{
				Description: "Command to run after the stack is deployed.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"extra_args": schema.ListAttribute{
				Description: "Extra arguments passed after docker compose up -d.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					useStateForUnknownIncludingNull{},
				},
			},
			"build_extra_args": schema.ListAttribute{
				Description: "Extra arguments passed after docker compose build.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					useStateForUnknownIncludingNull{},
				},
			},
			"compose_cmd_wrapper": schema.StringAttribute{
				Description: "Optional command wrapper invoked before docker compose (e.g. \"docker buildx bake\" or a custom script).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"compose_cmd_wrapper_include": schema.ListAttribute{
				Description: "Compose subcommands to wrap with compose_cmd_wrapper (e.g. [\"up\", \"build\"]).",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"ignore_services": schema.ListAttribute{
				Description: "Services to exclude from stack health status checks.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					useStateForUnknownIncludingNull{},
				},
			},
			"auto_update_skip_services": schema.ListAttribute{
				Description: "Services left out of the global auto-update. Manual update checks still include them. Leave it out and Terraform does not manage the list; set it to [] to clear it.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"config_files": schema.ListNestedAttribute{
				Description: "Extra files, besides the compose and env files, that Komodo tracks for this stack. A change to one counts as a change in DeployStackIfChanged. Env files passed with --env-file belong in additional_env_files instead. Leave it out and Terraform does not manage the list; set it to [] to clear it.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{Required: true, Description: "Path of the file, relative to run_directory."},
						"services": schema.ListAttribute{
							Optional: true, Computed: true, ElementType: types.StringType,
							Default:     listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})),
							Description: "Services that depend on the file. Empty means all services.",
						},
						"requires": schema.StringAttribute{
							Optional: true, Computed: true, Default: stringdefault.StaticString("None"),
							Description: "What a change to the file requires: \"Redeploy\", \"Restart\" or \"None\" (the default: track the file but take no action).",
						},
					},
				},
			},
			"send_alerts": schema.BoolAttribute{
				Description: "Whether to send StackStateChange alerts.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"started": schema.BoolAttribute{
				Description: "Whether the stack should be running. When true, runs DeployStack (compose up) to create and start containers. When false, runs DestroyStack (compose down) to stop and remove them. Intent-based and async.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"deployed": schema.BoolAttribute{
				Description: "When true, triggers a full DeployStack (git pull + compose up) after create/update. Use this to redeploy on config changes. Like started, this is intent-based and async.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"tags": schema.ListAttribute{
				Description: "List of tag names to apply to this stack.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *StackResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func stackConfigFromModel(ctx context.Context, m *StackResourceModel) client.StackConfig {
	serverID := m.ServerID.ValueString()
	if serverID == "" {
		serverID = "Local"
	}

	cfg := client.StackConfig{
		ServerID:              serverID,
		SwarmID:               m.SwarmID.ValueString(),
		ProjectName:           m.ProjectName.ValueString(),
		AutoPull:              client.BoolPtr(m.AutoPull.ValueBool()),
		RunBuild:              client.BoolPtr(m.RunBuild.ValueBool()),
		PollForUpdates:        client.BoolPtr(m.PollForUpdates.ValueBool()),
		AutoUpdate:            client.BoolPtr(m.AutoUpdate.ValueBool()),
		AutoUpdateAllServices: client.BoolPtr(m.AutoUpdateAllServices.ValueBool()),
		DestroyBeforeDeploy:   client.BoolPtr(m.DestroyBeforeDeploy.ValueBool()),
		SkipSecretInterp:      client.BoolPtr(m.SkipSecretInterp.ValueBool()),
		LinkedRepo:            m.LinkedRepo.ValueString(),
		GitProvider:           m.GitProvider.ValueString(),
		GitHTTPS:              client.BoolPtr(m.GitHTTPS.ValueBool()),
		GitAccount:            m.GitAccount.ValueString(),
		RegistryProvider:      m.RegistryProvider.ValueString(),
		RegistryAccount:       m.RegistryAccount.ValueString(),
		Repo:                  m.Repo.ValueString(),
		Branch:                m.Branch.ValueString(),
		Commit:                m.Commit.ValueString(),
		ClonePath:             m.ClonePath.ValueString(),
		Reclone:               client.BoolPtr(m.Reclone.ValueBool()),
		FilesOnHost:           client.BoolPtr(m.FilesOnHost.ValueBool()),
		FileContents:          m.FileContents.ValueString(),
		Environment:           m.Environment.ValueString(),
		RunDirectory:          m.RunDirectory.ValueString(),
		EnvFilePath:           m.EnvFilePath.ValueString(),
		WebhookEnabled:        client.BoolPtr(m.WebhookEnabled.ValueBool()),
		WebhookSecret:         m.WebhookSecret.ValueString(),
		WebhookForceDeploy:    client.BoolPtr(m.WebhookForceDeploy.ValueBool()),
		ComposeCmdWrapper:     m.ComposeCmdWrapper.ValueString(),
		SendAlerts:            client.BoolPtr(m.SendAlerts.ValueBool()),
	}

	if !m.Links.IsNull() && !m.Links.IsUnknown() {
		var v []string
		m.Links.ElementsAs(ctx, &v, false)
		cfg.Links = v
	}

	if !m.FilePaths.IsNull() {
		var paths []string
		m.FilePaths.ElementsAs(ctx, &paths, false)
		cfg.FilePaths = paths
	}

	if !m.AdditionalEnvFiles.IsNull() && !m.AdditionalEnvFiles.IsUnknown() {
		var rows []struct {
			Path  types.String `tfsdk:"path"`
			Track types.Bool   `tfsdk:"track"`
		}
		m.AdditionalEnvFiles.ElementsAs(ctx, &rows, false)
		out := make([]client.AdditionalEnvFile, 0, len(rows))
		for _, r := range rows {
			out = append(out, client.AdditionalEnvFile{
				Path:  r.Path.ValueString(),
				Track: r.Track.IsNull() || r.Track.ValueBool(),
			})
		}
		cfg.AdditionalEnvFiles = out
	}

	if !m.ExtraArgs.IsNull() {
		var v []string
		m.ExtraArgs.ElementsAs(ctx, &v, false)
		cfg.ExtraArgs = v
	}

	if !m.BuildExtraArgs.IsNull() {
		var v []string
		m.BuildExtraArgs.ElementsAs(ctx, &v, false)
		cfg.BuildExtraArgs = v
	}

	if !m.ComposeCmdWrapperInclude.IsNull() && !m.ComposeCmdWrapperInclude.IsUnknown() {
		var v []string
		m.ComposeCmdWrapperInclude.ElementsAs(ctx, &v, false)
		cfg.ComposeCmdWrapperInclude = v
	}

	if !m.IgnoreServices.IsNull() {
		var v []string
		m.IgnoreServices.ElementsAs(ctx, &v, false)
		cfg.IgnoreServices = v
	}

	// Null or unknown means "not managed": nothing is sent and Komodo keeps
	// what it has. An explicit empty list is sent and clears the field.
	if !m.AutoUpdateSkipServices.IsNull() && !m.AutoUpdateSkipServices.IsUnknown() {
		v := []string{}
		m.AutoUpdateSkipServices.ElementsAs(ctx, &v, false)
		cfg.AutoUpdateSkipServices = &v
	}
	if !m.ConfigFiles.IsNull() && !m.ConfigFiles.IsUnknown() {
		var rows []struct {
			Path     types.String `tfsdk:"path"`
			Services types.List   `tfsdk:"services"`
			Requires types.String `tfsdk:"requires"`
		}
		m.ConfigFiles.ElementsAs(ctx, &rows, false)
		out := make([]client.StackFileDependency, 0, len(rows))
		for _, r := range rows {
			dep := client.StackFileDependency{Path: r.Path.ValueString(), Requires: r.Requires.ValueString()}
			if !r.Services.IsNull() && !r.Services.IsUnknown() {
				r.Services.ElementsAs(ctx, &dep.Services, false)
			}
			out = append(out, dep)
		}
		cfg.ConfigFiles = &out
	}

	if m.PreDeployCommand.ValueString() != "" {
		cfg.PreDeploy = &client.SystemCommand{
			Path:    m.PreDeployPath.ValueString(),
			Command: m.PreDeployCommand.ValueString(),
		}
	}

	if m.PostDeployCommand.ValueString() != "" {
		cfg.PostDeploy = &client.SystemCommand{
			Path:    m.PostDeployPath.ValueString(),
			Command: m.PostDeployCommand.ValueString(),
		}
	}

	return cfg
}

// applyStartedState deploys or destroys the stack to match m.Started.
// DeployStack (compose up) creates and starts containers; DestroyStack (compose down) removes them.
// StartStack/StopStack are not used here because they only work on pre-existing containers.
func applyStartedState(ctx context.Context, c *client.Client, stackID string, m *StackResourceModel, diags *diag.Diagnostics) {
	if m.Started.ValueBool() {
		if err := c.DeployStack(ctx, stackID); err != nil {
			diags.AddError("Error starting stack", fmt.Sprintf("failed to deploy stack %q: %v", stackID, err))
		}
	} else {
		if err := c.DestroyStack(ctx, stackID); err != nil {
			diags.AddError("Error stopping stack", fmt.Sprintf("failed to destroy stack %q: %v", stackID, err))
		}
	}
}

// applyDeployedState triggers a full DeployStack (git pull + compose up) when m.Deployed is true.
// A short delay before the call lets Komodo finish registering the stack before accepting execute actions.
func applyDeployedState(ctx context.Context, c *client.Client, stackID string, m *StackResourceModel, diags *diag.Diagnostics) {
	if m.Deployed.ValueBool() {
		time.Sleep(5 * time.Second)
		if err := c.DeployStack(ctx, stackID); err != nil {
			diags.AddError("Error deploying stack", fmt.Sprintf("failed to deploy stack %q: %v", stackID, err))
		}
	}
}

func mapListToModel(vals []string, current types.List) types.List {
	if len(vals) > 0 {
		list, _ := types.ListValueFrom(context.Background(), types.StringType, vals)
		return list
	}
	if !current.IsNull() {
		return types.ListNull(types.StringType)
	}
	return current
}

// mapAdditionalEnvFilesToModel converts the API-side []AdditionalEnvFile back
// to the Terraform list-of-objects shape. Mirrors mapListToModel's null/empty
// handling so config that omits the attribute round-trips cleanly.
func mapAdditionalEnvFilesToModel(ctx context.Context, vals []client.AdditionalEnvFile, current types.List) types.List {
	if len(vals) == 0 {
		if current.IsNull() {
			return current
		}
		return types.ListNull(additionalEnvFileObjectType)
	}
	objs := make([]attr.Value, 0, len(vals))
	for _, v := range vals {
		o, diags := types.ObjectValue(additionalEnvFileObjectType.AttrTypes, map[string]attr.Value{
			"path":  types.StringValue(v.Path),
			"track": types.BoolValue(v.Track),
		})
		if diags.HasError() {
			continue
		}
		objs = append(objs, o)
	}
	list, _ := types.ListValue(additionalEnvFileObjectType, objs)
	return list
}

// mapStackConfigFilesToModel converts Komodo's config_files to the Terraform
// list-of-objects shape, but only while the attribute is managed (non-null in
// the model). A null attribute stays null whatever Komodo holds.
func mapStackConfigFilesToModel(vals *[]client.StackFileDependency, current types.List) types.List {
	if current.IsNull() || current.IsUnknown() {
		return types.ListNull(stackConfigFileObjectType)
	}
	if vals == nil || len(*vals) == 0 {
		return types.ListValueMust(stackConfigFileObjectType, []attr.Value{})
	}
	objs := make([]attr.Value, 0, len(*vals))
	for _, v := range *vals {
		services := v.Services
		if services == nil {
			services = []string{}
		}
		svcList, _ := types.ListValueFrom(context.Background(), types.StringType, services)
		requires := v.Requires
		if requires == "" {
			requires = "None" // Komodo omits the default
		}
		o, diags := types.ObjectValue(stackConfigFileObjectType.AttrTypes, map[string]attr.Value{
			"path":     types.StringValue(v.Path),
			"services": svcList,
			"requires": types.StringValue(requires),
		})
		if diags.HasError() {
			continue
		}
		objs = append(objs, o)
	}
	list, _ := types.ListValue(stackConfigFileObjectType, objs)
	return list
}

func mapStackToModel(ctx context.Context, c *client.Client, s *client.Stack, m *StackResourceModel) {
	m.ID = types.StringValue(s.ID)
	m.Name = types.StringValue(s.Name)
	m.ServerID = types.StringValue(preserveServerIDName(ctx, c, m.ServerID.ValueString(), s.Config.ServerID))
	m.SwarmID = types.StringValue(s.Config.SwarmID)
	m.LinkedRepo = types.StringValue(s.Config.LinkedRepo)
	m.ProjectName = types.StringValue(s.Config.ProjectName)
	m.GitProvider = types.StringValue(s.Config.GitProvider)
	m.GitAccount = types.StringValue(s.Config.GitAccount)
	m.RegistryProvider = types.StringValue(s.Config.RegistryProvider)
	m.RegistryAccount = types.StringValue(s.Config.RegistryAccount)
	m.Repo = types.StringValue(s.Config.Repo)
	m.Branch = types.StringValue(s.Config.Branch)
	m.Commit = types.StringValue(s.Config.Commit)
	m.ClonePath = types.StringValue(s.Config.ClonePath)
	m.ComposeCmdWrapper = types.StringValue(s.Config.ComposeCmdWrapper)
	// file_contents is not overridden from the API — the .tf config is authoritative.
	// The API may normalize whitespace/newlines, which would cause perpetual diffs.
	m.Environment = types.StringValue(strings.TrimRight(s.Config.Environment, "\n"))
	m.RunDirectory = types.StringValue(s.Config.RunDirectory)
	m.EnvFilePath = types.StringValue(s.Config.EnvFilePath)
	m.WebhookSecret = types.StringValue(s.Config.WebhookSecret)

	m.FilePaths = mapListToModel(s.Config.FilePaths, m.FilePaths)
	m.Links = mapListToModel(s.Config.Links, m.Links)
	m.ExtraArgs = mapListToModel(s.Config.ExtraArgs, m.ExtraArgs)
	m.BuildExtraArgs = mapListToModel(s.Config.BuildExtraArgs, m.BuildExtraArgs)
	m.ComposeCmdWrapperInclude = mapListToModel(s.Config.ComposeCmdWrapperInclude, m.ComposeCmdWrapperInclude)
	m.IgnoreServices = mapListToModel(s.Config.IgnoreServices, m.IgnoreServices)
	// Unmanaged (null) stays null; otherwise show what Komodo has.
	if m.AutoUpdateSkipServices.IsNull() || m.AutoUpdateSkipServices.IsUnknown() {
		m.AutoUpdateSkipServices = types.ListNull(types.StringType)
	} else if s.Config.AutoUpdateSkipServices != nil && len(*s.Config.AutoUpdateSkipServices) > 0 {
		m.AutoUpdateSkipServices, _ = types.ListValueFrom(ctx, types.StringType, *s.Config.AutoUpdateSkipServices)
	} else {
		m.AutoUpdateSkipServices = types.ListValueMust(types.StringType, []attr.Value{})
	}
	m.ConfigFiles = mapStackConfigFilesToModel(s.Config.ConfigFiles, m.ConfigFiles)
	m.AdditionalEnvFiles = mapAdditionalEnvFilesToModel(ctx, s.Config.AdditionalEnvFiles, m.AdditionalEnvFiles)

	if s.Config.AutoPull != nil {
		m.AutoPull = types.BoolValue(*s.Config.AutoPull)
	}
	if s.Config.RunBuild != nil {
		m.RunBuild = types.BoolValue(*s.Config.RunBuild)
	}
	if s.Config.PollForUpdates != nil {
		m.PollForUpdates = types.BoolValue(*s.Config.PollForUpdates)
	}
	if s.Config.AutoUpdate != nil {
		m.AutoUpdate = types.BoolValue(*s.Config.AutoUpdate)
	}
	if s.Config.AutoUpdateAllServices != nil {
		m.AutoUpdateAllServices = types.BoolValue(*s.Config.AutoUpdateAllServices)
	}
	if s.Config.DestroyBeforeDeploy != nil {
		m.DestroyBeforeDeploy = types.BoolValue(*s.Config.DestroyBeforeDeploy)
	}
	if s.Config.SkipSecretInterp != nil {
		m.SkipSecretInterp = types.BoolValue(*s.Config.SkipSecretInterp)
	}
	if s.Config.Reclone != nil {
		m.Reclone = types.BoolValue(*s.Config.Reclone)
	}
	if s.Config.GitHTTPS != nil {
		m.GitHTTPS = types.BoolValue(*s.Config.GitHTTPS)
	}
	if s.Config.FilesOnHost != nil {
		m.FilesOnHost = types.BoolValue(*s.Config.FilesOnHost)
	}
	if s.Config.WebhookEnabled != nil {
		m.WebhookEnabled = types.BoolValue(*s.Config.WebhookEnabled)
	}
	if s.Config.WebhookForceDeploy != nil {
		m.WebhookForceDeploy = types.BoolValue(*s.Config.WebhookForceDeploy)
	}
	if s.Config.SendAlerts != nil {
		m.SendAlerts = types.BoolValue(*s.Config.SendAlerts)
	}

	if s.Config.PreDeploy != nil {
		m.PreDeployPath = types.StringValue(strings.TrimRight(s.Config.PreDeploy.Path, "\n"))
		m.PreDeployCommand = types.StringValue(strings.TrimRight(s.Config.PreDeploy.Command, "\n"))
	}
	if s.Config.PostDeploy != nil {
		m.PostDeployPath = types.StringValue(strings.TrimRight(s.Config.PostDeploy.Path, "\n"))
		m.PostDeployCommand = types.StringValue(strings.TrimRight(s.Config.PostDeploy.Command, "\n"))
	}

	m.Tags = mapTagsToModel(ctx, c, s.Tags, m.Tags)
}

func (r *StackResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan StackResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := stackConfigFromModel(ctx, &plan)
	cfg.ServerID = resolveServerID(ctx, r.client, plan.ServerID.ValueString())

	stack, err := r.client.CreateStack(ctx, client.CreateStackParams{
		Name:   plan.Name.ValueString(),
		Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating stack",
			fmt.Sprintf("failed creating stack %q on server %q: %v", plan.Name.ValueString(), plan.ServerID.ValueString(), err),
		)
		return
	}

	tagsFromPlan := plan.Tags
	mapStackToModel(ctx, r.client, stack, &plan)
	applyResourceTags(ctx, r.client, "Stack", stack.ID, tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}

	// Deploy first (git pull + compose up), then ensure started state.
	// If deployed already triggered a DeployStack, skip applyStartedState to
	// avoid a "Resource is busy" error from double-calling the API.
	applyDeployedState(ctx, r.client, stack.ID, &plan, &resp.Diagnostics)
	if !plan.Deployed.ValueBool() {
		applyStartedState(ctx, r.client, stack.ID, &plan, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *StackResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state StackResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() || state.ID.IsUnknown() || state.ID.ValueString() == "" {
		// If we don't have an ID yet, the resource isn't created yet.
		resp.State.RemoveResource(ctx)
		return
	}

	if state.ServerID.ValueString() == "" {
		state.ServerID = types.StringValue("Local")
	}

	stack, err := r.client.GetStack(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading stack",
			fmt.Sprintf("failed reading stack %q (id=%q) from server %q: %v", state.Name.ValueString(), state.ID.ValueString(), state.ServerID.ValueString(), err),
		)
		return
	}

	mapStackToModel(ctx, r.client, stack, &state)
	// `started` is an intent attribute — leave it as whatever is in state.
	// StartStack/StopStack are async so reading container state immediately
	// after would race. Drift is surfaced the next time apply is run.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *StackResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan StackResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state StackResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Name.ValueString() != state.Name.ValueString() {
		err := r.client.Write(ctx, "RenameStack", map[string]string{
			"id": state.ID.ValueString(), "name": plan.Name.ValueString(),
		}, nil)
		if err != nil {
			resp.Diagnostics.AddError("Error renaming stack", err.Error())
			return
		}
	}

	cfg := stackConfigFromModel(ctx, &plan)
	cfg.ServerID = resolveServerID(ctx, r.client, plan.ServerID.ValueString())

	stack, err := r.client.UpdateStack(ctx, client.UpdateStackParams{
		ID:     state.ID.ValueString(),
		Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating stack",
			fmt.Sprintf("failed updating stack %q (id=%q) on server %q: %v", plan.Name.ValueString(), state.ID.ValueString(), plan.ServerID.ValueString(), err),
		)
		return
	}

	tagsFromPlan := plan.Tags
	mapStackToModel(ctx, r.client, stack, &plan)
	applyResourceTags(ctx, r.client, "Stack", state.ID.ValueString(), tagsFromPlan, &resp.Diagnostics)
	if !tagsFromPlan.IsUnknown() {
		plan.Tags = tagsFromPlan
	}

	// Deploy first (git pull + compose up), then ensure started state.
	// If deployed already triggered a DeployStack, skip applyStartedState to
	// avoid a "Resource is busy" error from double-calling the API.
	applyDeployedState(ctx, r.client, state.ID.ValueString(), &plan, &resp.Diagnostics)
	if !plan.Deployed.ValueBool() {
		applyStartedState(ctx, r.client, state.ID.ValueString(), &plan, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *StackResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state StackResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteStack(ctx, state.ID.ValueString())
	if err != nil {
		var nfe *client.NotFoundError
		if errors.As(err, &nfe) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting stack",
			fmt.Sprintf("failed deleting stack %q (id=%q) from server %q: %v", state.Name.ValueString(), state.ID.ValueString(), state.ServerID.ValueString(), err),
		)
	}
}

func (r *StackResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *StackResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip on destroy or when client is not yet configured.
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}
	var plan StackResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// server_id drift handled by Create/Update + preserveServerIDName.
	changed := false

	// Normalize empty lists to null so they round-trip consistently
	// (the API treats [] and unset identically).
	for _, lp := range []*types.List{
		&plan.FilePaths, &plan.Links,
		&plan.ExtraArgs, &plan.BuildExtraArgs,
		&plan.ComposeCmdWrapperInclude, &plan.IgnoreServices,
	} {
		if !lp.IsNull() && !lp.IsUnknown() && len(lp.Elements()) == 0 {
			*lp = types.ListNull(types.StringType)
			changed = true
		}
	}
	if !plan.AdditionalEnvFiles.IsNull() && !plan.AdditionalEnvFiles.IsUnknown() && len(plan.AdditionalEnvFiles.Elements()) == 0 {
		plan.AdditionalEnvFiles = types.ListNull(additionalEnvFileObjectType)
		changed = true
	}

	if changed {
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	}
}

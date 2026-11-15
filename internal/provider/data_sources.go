package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/johnciavarella/terraform-provider-komodo/internal/client"
)

// ──────────────────────────────── Server ────────────────────────────────

var _ datasource.DataSource = &ServerDataSource{}

type ServerDataSource struct{ client *client.Client }

type ServerDataSourceModel struct {
	ID                 types.String  `tfsdk:"id"`
	Name               types.String  `tfsdk:"name"`
	Address            types.String  `tfsdk:"address"`
	ExternalAddress    types.String  `tfsdk:"external_address"`
	Region             types.String  `tfsdk:"region"`
	Enabled            types.Bool    `tfsdk:"enabled"`
	InsecureTLS        types.Bool    `tfsdk:"insecure_tls"`
	AutoRotateKeys     types.Bool    `tfsdk:"auto_rotate_keys"`
	PeripheryPublicKey types.String  `tfsdk:"periphery_public_key"`
	Links              types.List    `tfsdk:"links"`
	StatsMonitoring    types.Bool    `tfsdk:"stats_monitoring"`
	AutoPrune          types.Bool    `tfsdk:"auto_prune"`
	CPUWarning         types.Float64 `tfsdk:"cpu_warning"`
	CPUCritical        types.Float64 `tfsdk:"cpu_critical"`
	MemWarning         types.Float64 `tfsdk:"mem_warning"`
	MemCritical        types.Float64 `tfsdk:"mem_critical"`
	DiskWarning        types.Float64 `tfsdk:"disk_warning"`
	DiskCritical       types.Float64 `tfsdk:"disk_critical"`
}

func NewServerDataSource() datasource.DataSource { return &ServerDataSource{} }

func (d *ServerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (d *ServerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo server by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Required: true, Description: "Server ID or name."},
			"name":                 schema.StringAttribute{Computed: true, Description: "Server name."},
			"address":              schema.StringAttribute{Computed: true},
			"external_address":     schema.StringAttribute{Computed: true},
			"region":               schema.StringAttribute{Computed: true},
			"enabled":              schema.BoolAttribute{Computed: true},
			"insecure_tls":         schema.BoolAttribute{Computed: true},
			"auto_rotate_keys":     schema.BoolAttribute{Computed: true},
			"periphery_public_key": schema.StringAttribute{Computed: true},
			"links":                schema.ListAttribute{Computed: true, ElementType: types.StringType},
			"stats_monitoring":     schema.BoolAttribute{Computed: true},
			"auto_prune":           schema.BoolAttribute{Computed: true},
			"cpu_warning":          schema.Float64Attribute{Computed: true},
			"cpu_critical":         schema.Float64Attribute{Computed: true},
			"mem_warning":          schema.Float64Attribute{Computed: true},
			"mem_critical":         schema.Float64Attribute{Computed: true},
			"disk_warning":         schema.Float64Attribute{Computed: true},
			"disk_critical":        schema.Float64Attribute{Computed: true},
		},
	}
}

func (d *ServerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *ServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ServerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	server, err := d.client.GetServer(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading server", err.Error())
		return
	}

	config.ID = types.StringValue(server.ID)
	config.Name = types.StringValue(server.Name)
	config.Address = types.StringValue(server.Config.Address)
	config.ExternalAddress = types.StringValue(server.Config.ExternalAddress)
	config.Region = types.StringValue(server.Config.Region)
	config.PeripheryPublicKey = types.StringValue(server.PeripheryPublicKey)
	if len(server.Config.Links) > 0 {
		list, _ := types.ListValueFrom(ctx, types.StringType, server.Config.Links)
		config.Links = list
	} else {
		config.Links = types.ListNull(types.StringType)
	}
	if server.Config.Enabled != nil {
		config.Enabled = types.BoolValue(*server.Config.Enabled)
	}
	if server.Config.InsecureTLS != nil {
		config.InsecureTLS = types.BoolValue(*server.Config.InsecureTLS)
	}
	if server.Config.AutoRotateKeys != nil {
		config.AutoRotateKeys = types.BoolValue(*server.Config.AutoRotateKeys)
	}
	if server.Config.StatsMonitoring != nil {
		config.StatsMonitoring = types.BoolValue(*server.Config.StatsMonitoring)
	}
	if server.Config.AutoPrune != nil {
		config.AutoPrune = types.BoolValue(*server.Config.AutoPrune)
	}
	if server.Config.CPUWarning != nil {
		config.CPUWarning = types.Float64Value(*server.Config.CPUWarning)
	}
	if server.Config.CPUCritical != nil {
		config.CPUCritical = types.Float64Value(*server.Config.CPUCritical)
	}
	if server.Config.MemWarning != nil {
		config.MemWarning = types.Float64Value(*server.Config.MemWarning)
	}
	if server.Config.MemCritical != nil {
		config.MemCritical = types.Float64Value(*server.Config.MemCritical)
	}
	if server.Config.DiskWarning != nil {
		config.DiskWarning = types.Float64Value(*server.Config.DiskWarning)
	}
	if server.Config.DiskCritical != nil {
		config.DiskCritical = types.Float64Value(*server.Config.DiskCritical)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ──────────────────────────────── Stack ────────────────────────────────

var _ datasource.DataSource = &StackDataSource{}

type StackDataSource struct{ client *client.Client }

type StackDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ServerID       types.String `tfsdk:"server_id"`
	SwarmID        types.String `tfsdk:"swarm_id"`
	LinkedRepo     types.String `tfsdk:"linked_repo"`
	ProjectName    types.String `tfsdk:"project_name"`
	Repo           types.String `tfsdk:"repo"`
	Branch         types.String `tfsdk:"branch"`
	GitProvider    types.String `tfsdk:"git_provider"`
	IgnoreServices types.List   `tfsdk:"ignore_services"`
}

func NewStackDataSource() datasource.DataSource { return &StackDataSource{} }

func (d *StackDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stack"
}

func (d *StackDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo stack by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true, Description: "Stack ID or name."},
			"name":         schema.StringAttribute{Computed: true},
			"server_id":    schema.StringAttribute{Computed: true},
			"swarm_id":     schema.StringAttribute{Computed: true},
			"linked_repo":  schema.StringAttribute{Computed: true},
			"project_name": schema.StringAttribute{Computed: true},
			"repo":         schema.StringAttribute{Computed: true},
			"branch":       schema.StringAttribute{Computed: true},
			"git_provider": schema.StringAttribute{Computed: true},
			"ignore_services": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Services excluded from stack health status checks.",
			},
		},
	}
}

func (d *StackDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *StackDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config StackDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stack, err := d.client.GetStack(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading stack", err.Error())
		return
	}

	config.ID = types.StringValue(stack.ID)
	config.Name = types.StringValue(stack.Name)
	config.ServerID = types.StringValue(stack.Config.ServerID)
	config.SwarmID = types.StringValue(stack.Config.SwarmID)
	config.LinkedRepo = types.StringValue(stack.Config.LinkedRepo)
	config.ProjectName = types.StringValue(stack.Config.ProjectName)
	config.Repo = types.StringValue(stack.Config.Repo)
	config.Branch = types.StringValue(stack.Config.Branch)
	config.GitProvider = types.StringValue(stack.Config.GitProvider)

	ignoreServices, diags := types.ListValueFrom(ctx, types.StringType, stack.Config.IgnoreServices)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.IgnoreServices = ignoreServices

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ──────────────────────────────── Deployment ────────────────────────────────

var _ datasource.DataSource = &DeploymentDataSource{}

type DeploymentDataSource struct{ client *client.Client }

type DeploymentDataSourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	ServerID             types.String `tfsdk:"server_id"`
	SwarmID              types.String `tfsdk:"swarm_id"`
	ImageType            types.String `tfsdk:"image_type"`
	Image                types.String `tfsdk:"image"`
	ImageRegistryAccount types.String `tfsdk:"image_registry_account"`
	Network              types.String `tfsdk:"network"`
	RestartMode          types.String `tfsdk:"restart_mode"`
}

func NewDeploymentDataSource() datasource.DataSource { return &DeploymentDataSource{} }

func (d *DeploymentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (d *DeploymentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo deployment by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":                     schema.StringAttribute{Required: true, Description: "Deployment ID or name."},
			"name":                   schema.StringAttribute{Computed: true},
			"server_id":              schema.StringAttribute{Computed: true},
			"swarm_id":               schema.StringAttribute{Computed: true},
			"image_type":             schema.StringAttribute{Computed: true},
			"image":                  schema.StringAttribute{Computed: true},
			"image_registry_account": schema.StringAttribute{Computed: true},
			"network":                schema.StringAttribute{Computed: true},
			"restart_mode":           schema.StringAttribute{Computed: true},
		},
	}
}

func (d *DeploymentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *DeploymentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DeploymentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deployment, err := d.client.GetDeployment(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading deployment", err.Error())
		return
	}

	config.ID = types.StringValue(deployment.ID)
	config.Name = types.StringValue(deployment.Name)
	config.ServerID = types.StringValue(deployment.Config.ServerID)
	config.SwarmID = types.StringValue(deployment.Config.SwarmID)
	config.ImageType = types.StringValue(deployment.Config.Image.Type)
	if deployment.Config.Image.Type == "Build" {
		config.Image = types.StringValue(deployment.Config.Image.Params.BuildID)
	} else {
		config.Image = types.StringValue(deployment.Config.Image.Params.Image)
	}
	config.ImageRegistryAccount = types.StringValue(deployment.Config.ImageRegistryAccount)
	config.Network = types.StringValue(deployment.Config.Network)
	config.RestartMode = types.StringValue(deployment.Config.RestartMode)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ──────────────────────────────── Build ────────────────────────────────

var _ datasource.DataSource = &BuildDataSource{}

type BuildDataSource struct{ client *client.Client }

type BuildDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	BuilderID   types.String `tfsdk:"builder_id"`
	ImageName   types.String `tfsdk:"image_name"`
	LinkedRepo  types.String `tfsdk:"linked_repo"`
	Repo        types.String `tfsdk:"repo"`
	Branch      types.String `tfsdk:"branch"`
	GitProvider types.String `tfsdk:"git_provider"`
}

func NewBuildDataSource() datasource.DataSource { return &BuildDataSource{} }

func (d *BuildDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_build"
}

func (d *BuildDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo build by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true, Description: "Build ID or name."},
			"name":         schema.StringAttribute{Computed: true},
			"builder_id":   schema.StringAttribute{Computed: true},
			"image_name":   schema.StringAttribute{Computed: true},
			"linked_repo":  schema.StringAttribute{Computed: true},
			"repo":         schema.StringAttribute{Computed: true},
			"branch":       schema.StringAttribute{Computed: true},
			"git_provider": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *BuildDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *BuildDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BuildDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	build, err := d.client.GetBuild(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading build", err.Error())
		return
	}

	config.ID = types.StringValue(build.ID)
	config.Name = types.StringValue(build.Name)
	config.BuilderID = types.StringValue(build.Config.BuilderID)
	config.ImageName = types.StringValue(build.Config.ImageName)
	config.LinkedRepo = types.StringValue(build.Config.LinkedRepo)
	config.Repo = types.StringValue(build.Config.Repo)
	config.Branch = types.StringValue(build.Config.Branch)
	config.GitProvider = types.StringValue(build.Config.GitProvider)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ──────────────────────────────── Repo ────────────────────────────────

var _ datasource.DataSource = &RepoDataSource{}

type RepoDataSource struct{ client *client.Client }

type RepoDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ServerID    types.String `tfsdk:"server_id"`
	BuilderID   types.String `tfsdk:"builder_id"`
	Repo        types.String `tfsdk:"repo"`
	Branch      types.String `tfsdk:"branch"`
	Path        types.String `tfsdk:"path"`
	GitProvider types.String `tfsdk:"git_provider"`
}

func NewRepoDataSource() datasource.DataSource { return &RepoDataSource{} }

func (d *RepoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_repo"
}

func (d *RepoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo repo by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true, Description: "Repo ID or name."},
			"name":         schema.StringAttribute{Computed: true},
			"server_id":    schema.StringAttribute{Computed: true},
			"builder_id":   schema.StringAttribute{Computed: true},
			"repo":         schema.StringAttribute{Computed: true},
			"branch":       schema.StringAttribute{Computed: true},
			"path":         schema.StringAttribute{Computed: true},
			"git_provider": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *RepoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *RepoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RepoDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repo, err := d.client.GetRepo(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading repo", err.Error())
		return
	}

	config.ID = types.StringValue(repo.ID)
	config.Name = types.StringValue(repo.Name)
	config.ServerID = types.StringValue(repo.Config.ServerID)
	config.BuilderID = types.StringValue(repo.Config.BuilderID)
	config.Repo = types.StringValue(repo.Config.Repo)
	config.Branch = types.StringValue(repo.Config.Branch)
	config.Path = types.StringValue(repo.Config.Path)
	config.GitProvider = types.StringValue(repo.Config.GitProvider)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ──────────────────────────────── Tag ────────────────────────────────

var _ datasource.DataSource = &TagDataSource{}

type TagDataSource struct{ client *client.Client }

type TagDataSourceModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Color types.String `tfsdk:"color"`
}

func NewTagDataSource() datasource.DataSource { return &TagDataSource{} }

func (d *TagDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (d *TagDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo tag by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Required: true, Description: "Tag ID or name."},
			"name":  schema.StringAttribute{Computed: true},
			"color": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *TagDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *TagDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TagDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tag, err := d.client.GetTag(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading tag", err.Error())
		return
	}

	config.ID = types.StringValue(tag.ID)
	config.Name = types.StringValue(tag.Name)
	config.Color = types.StringValue(tag.Color)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ──────────────────────────────── Builder ────────────────────────────────

var _ datasource.DataSource = &BuilderDataSource{}

type BuilderDataSource struct{ client *client.Client }

type BuilderDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	ServerID types.String `tfsdk:"server_id"`
}

func NewBuilderDataSource() datasource.DataSource { return &BuilderDataSource{} }

func (d *BuilderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_builder"
}

func (d *BuilderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo builder by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Required: true, Description: "Builder ID or name."},
			"name":      schema.StringAttribute{Computed: true},
			"server_id": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *BuilderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *BuilderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BuilderDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	builder, err := d.client.GetBuilder(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading builder", err.Error())
		return
	}

	config.ID = types.StringValue(builder.ID)
	config.Name = types.StringValue(builder.Name)
	config.ServerID = types.StringValue(builder.Config.ServerID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ---- DockerRegistryAccount data source ----

type DockerRegistryAccountDataSource struct {
	client *client.Client
}

type DockerRegistryAccountDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Domain   types.String `tfsdk:"domain"`
	Username types.String `tfsdk:"username"`
}

func NewDockerRegistryAccountDataSource() datasource.DataSource {
	return &DockerRegistryAccountDataSource{}
}

func (d *DockerRegistryAccountDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_docker_registry_account"
}

func (d *DockerRegistryAccountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a Komodo docker registry account by ID.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Required: true, Description: "Docker registry account ID."},
			"domain":   schema.StringAttribute{Computed: true, Description: "Registry domain."},
			"username": schema.StringAttribute{Computed: true, Description: "Registry account username."},
		},
	}
}

func (d *DockerRegistryAccountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *DockerRegistryAccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DockerRegistryAccountDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account, err := d.client.GetDockerRegistryAccount(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading docker registry account", err.Error())
		return
	}

	config.ID = types.StringValue(account.ID)
	config.Domain = types.StringValue(account.Domain)
	config.Username = types.StringValue(account.Username)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

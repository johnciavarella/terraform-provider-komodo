package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NotFoundError signals a 404 or missing resource.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// APIError represents an error response from the Komodo API.
type APIError struct {
	Error string   `json:"error"`
	Trace []string `json:"trace,omitempty"`
}

// Client wraps the Komodo Core API.
type Client struct {
	BaseURL    string
	APIKey     string
	APISecret  string
	HTTPClient *http.Client
	UserAgent  string
}

// NewClient constructs a Komodo API client.
// insecureSkipVerify disables TLS certificate verification — only use for
// local-test instances behind self-signed certs.
func NewClient(baseURL, apiKey, apiSecret, userAgent string, insecureSkipVerify bool) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		BaseURL:   baseURL,
		APIKey:    apiKey,
		APISecret: apiSecret,
		HTTPClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
		UserAgent: userAgent,
	}
}

// rpcRequest is the common body shape for Komodo RPC calls.
type rpcRequest struct {
	Type   string      `json:"type"`
	Params interface{} `json:"params"`
}

// doRPC sends an RPC request to the given path (/read, /write, /execute).
func (c *Client) doRPC(ctx context.Context, path string, reqType string, params interface{}, result interface{}) error {
	body := rpcRequest{
		Type:   reqType,
		Params: params,
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling request body: %w", err)
	}

	// Retry loop for transient errors.
	var resp *http.Response
	maxRetries := 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", c.APIKey)
		req.Header.Set("X-Api-Secret", c.APISecret)
		req.Header.Set("User-Agent", c.UserAgent)

		resp, err = c.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("executing request: %w", err)
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			// Only close the body when we are going to retry, otherwise leave it open for the deferred close below.
			if attempt < maxRetries {
				resp.Body.Close()
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
		}
		break
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return &NotFoundError{Message: "resource not found"}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, apiErr.Error)
		}
		// Truncate raw body to avoid leaking sensitive data from unexpected responses.
		msg := string(respBody)
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, msg)
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshalling response: %w", err)
		}
	}

	return nil
}

// Read sends a request to the /read endpoint.
func (c *Client) Read(ctx context.Context, reqType string, params interface{}, result interface{}) error {
	return c.doRPC(ctx, "/read", reqType, params, result)
}

// Write sends a request to the /write endpoint.
func (c *Client) Write(ctx context.Context, reqType string, params interface{}, result interface{}) error {
	return c.doRPC(ctx, "/write", reqType, params, result)
}

// Execute sends a request to the /execute endpoint.
func (c *Client) Execute(ctx context.Context, reqType string, params interface{}, result interface{}) error {
	return c.doRPC(ctx, "/execute", reqType, params, result)
}

// ---- Common types ----

// parseMongoID extracts the hex string from a Komodo API ID field.
// The API returns IDs as MongoDB ObjectId objects: {"$oid": "hexstring"}.
// It also handles the case where the value is already a plain string.
func parseMongoID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var oid struct {
		OID string `json:"$oid"`
	}
	if json.Unmarshal(raw, &oid) == nil {
		return oid.OID
	}
	return ""
}

// Resource is the common base for all Komodo resources.
type Resource struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Tags   []string        `json:"tags,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
	Info   json.RawMessage `json:"info,omitempty"`
}

// ---- Server ----

// MaintenanceWindow is a scheduled period during which alerts are suppressed.
// ScheduleType is "Daily", "Weekly" or "OneTime".
type MaintenanceWindow struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	ScheduleType    string `json:"schedule_type"`
	DayOfWeek       string `json:"day_of_week"`
	Date            string `json:"date"`
	Hour            int    `json:"hour"`
	Minute          int    `json:"minute"`
	DurationMinutes int    `json:"duration_minutes"`
	Timezone        string `json:"timezone"`
	Enabled         bool   `json:"enabled"`
}

type ServerConfig struct {
	Address                   string   `json:"address,omitempty"`
	ExternalAddress           string   `json:"external_address,omitempty"`
	Region                    string   `json:"region,omitempty"`
	Enabled                   *bool    `json:"enabled,omitempty"`
	Passkey                   string   `json:"passkey,omitempty"`
	InsecureTLS               *bool    `json:"insecure_tls,omitempty"`
	AutoRotateKeys            *bool    `json:"auto_rotate_keys,omitempty"`
	IgnoreMounts              []string `json:"ignore_mounts,omitempty"`
	Links                     []string `json:"links,omitempty"`
	StatsMonitoring           *bool    `json:"stats_monitoring,omitempty"`
	AutoPrune                 *bool    `json:"auto_prune,omitempty"`
	SendUnreachableAlerts     *bool    `json:"send_unreachable_alerts,omitempty"`
	SendCPUAlerts             *bool    `json:"send_cpu_alerts,omitempty"`
	SendMemAlerts             *bool    `json:"send_mem_alerts,omitempty"`
	SendDiskAlerts            *bool    `json:"send_disk_alerts,omitempty"`
	SendVersionMismatchAlerts *bool    `json:"send_version_mismatch_alerts,omitempty"`
	// Pointer to a slice: nil leaves the windows alone, empty clears them.
	MaintenanceWindows *[]MaintenanceWindow `json:"maintenance_windows,omitempty"`
	CPUWarning         *float64             `json:"cpu_warning,omitempty"`
	CPUCritical        *float64             `json:"cpu_critical,omitempty"`
	MemWarning         *float64             `json:"mem_warning,omitempty"`
	MemCritical        *float64             `json:"mem_critical,omitempty"`
	DiskWarning        *float64             `json:"disk_warning,omitempty"`
	DiskCritical       *float64             `json:"disk_critical,omitempty"`
}

type Server struct {
	ID                 string       `json:"-"`
	Name               string       `json:"name"`
	Tags               []string     `json:"tags,omitempty"`
	Config             ServerConfig `json:"config"`
	PeripheryPublicKey string       `json:"-"`
}

func (s *Server) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID                 json.RawMessage `json:"_id"`
		Name               string          `json:"name"`
		Tags               []string        `json:"tags,omitempty"`
		Config             ServerConfig    `json:"config"`
		PeripheryPublicKey string          `json:"periphery_public_key,omitempty"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	s.ID = parseMongoID(a.ID)
	s.Name = a.Name
	s.Tags = a.Tags
	s.Config = a.Config
	s.PeripheryPublicKey = a.PeripheryPublicKey
	return nil
}

type CreateServerParams struct {
	Name   string       `json:"name"`
	Config ServerConfig `json:"config,omitempty"`
}

type UpdateServerParams struct {
	ID     string       `json:"id"`
	Config ServerConfig `json:"config"`
}

type IDParam struct {
	ID string `json:"id"`
}

type NameOrIDParam struct {
	Server     string `json:"server,omitempty"`
	Stack      string `json:"stack,omitempty"`
	Deployment string `json:"deployment,omitempty"`
	Build      string `json:"build,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Builder    string `json:"builder,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

func (c *Client) CreateServer(ctx context.Context, params CreateServerParams) (*Server, error) {
	var s Server
	err := c.Write(ctx, "CreateServer", params, &s)
	return &s, err
}

func (c *Client) GetServer(ctx context.Context, idOrName string) (*Server, error) {
	if idOrName == "" {
		return nil, &NotFoundError{Message: "server identifier is empty"}
	}
	var s Server
	err := c.Read(ctx, "GetServer", map[string]string{"server": idOrName}, &s)
	return &s, err
}

func (c *Client) UpdateServer(ctx context.Context, params UpdateServerParams) (*Server, error) {
	var s Server
	err := c.Write(ctx, "UpdateServer", params, &s)
	return &s, err
}

func (c *Client) DeleteServer(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteServer", IDParam{ID: id}, nil)
}

func (c *Client) UpdateServerPublicKey(ctx context.Context, server, publicKey string) error {
	return c.Write(ctx, "UpdateServerPublicKey", map[string]string{
		"server":     server,
		"public_key": publicKey,
	}, nil)
}

func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	var servers []Server
	err := c.Read(ctx, "ListServers", struct{}{}, &servers)
	return servers, err
}

// ---- Stack ----

// SystemCommand represents a shell command with an optional working directory.
type SystemCommand struct {
	Path    string `json:"path"`
	Command string `json:"command"`
}

// AdditionalEnvFile is the v2 shape of stack additional_env_files entries
// (was a plain string in v1). The Komodo backend's custom deserializer
// also accepts a bare string for backwards compatibility on read.
type AdditionalEnvFile struct {
	Path  string `json:"path"`
	Track bool   `json:"track"`
}

// UnmarshalJSON tolerates legacy v1 payloads where additional_env_files
// elements were plain strings.
func (a *AdditionalEnvFile) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		a.Path = s
		a.Track = true
		return nil
	}
	type alias AdditionalEnvFile
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*a = AdditionalEnvFile(v)
	return nil
}

// StackFileDependency is an extra file Komodo tracks for a stack. A change to
// it counts as a stack change in DeployStackIfChanged. Requires is one of
// "Redeploy", "Restart" or "None"; Komodo omits it when it is "None".
type StackFileDependency struct {
	Path     string   `json:"path"`
	Services []string `json:"services,omitempty"`
	Requires string   `json:"requires,omitempty"`
}

type StackConfig struct {
	ServerID                 string              `json:"server_id,omitempty"`
	SwarmID                  string              `json:"swarm_id,omitempty"`
	Links                    []string            `json:"links,omitempty"`
	ProjectName              string              `json:"project_name,omitempty"`
	AutoPull                 *bool               `json:"auto_pull,omitempty"`
	RunBuild                 *bool               `json:"run_build,omitempty"`
	PollForUpdates           *bool               `json:"poll_for_updates,omitempty"`
	AutoUpdate               *bool               `json:"auto_update,omitempty"`
	AutoUpdateAllServices    *bool               `json:"auto_update_all_services,omitempty"`
	DestroyBeforeDeploy      *bool               `json:"destroy_before_deploy,omitempty"`
	SkipSecretInterp         *bool               `json:"skip_secret_interp,omitempty"`
	LinkedRepo               string              `json:"linked_repo,omitempty"`
	GitProvider              string              `json:"git_provider,omitempty"`
	GitHTTPS                 *bool               `json:"git_https,omitempty"`
	GitAccount               string              `json:"git_account,omitempty"`
	RegistryProvider         string              `json:"registry_provider,omitempty"`
	RegistryAccount          string              `json:"registry_account,omitempty"`
	Repo                     string              `json:"repo,omitempty"`
	Branch                   string              `json:"branch,omitempty"`
	Commit                   string              `json:"commit,omitempty"`
	ClonePath                string              `json:"clone_path,omitempty"`
	Reclone                  *bool               `json:"reclone,omitempty"`
	FilesOnHost              *bool               `json:"files_on_host,omitempty"`
	RunDirectory             string              `json:"run_directory,omitempty"`
	FilePaths                []string            `json:"file_paths,omitempty"`
	FileContents             string              `json:"file_contents,omitempty"`
	Environment              string              `json:"environment,omitempty"`
	EnvFilePath              string              `json:"env_file_path,omitempty"`
	AdditionalEnvFiles       []AdditionalEnvFile `json:"additional_env_files,omitempty"`
	WebhookEnabled           *bool               `json:"webhook_enabled,omitempty"`
	WebhookSecret            string              `json:"webhook_secret,omitempty"`
	WebhookForceDeploy       *bool               `json:"webhook_force_deploy,omitempty"`
	PreDeploy                *SystemCommand      `json:"pre_deploy,omitempty"`
	PostDeploy               *SystemCommand      `json:"post_deploy,omitempty"`
	ExtraArgs                []string            `json:"extra_args,omitempty"`
	BuildExtraArgs           []string            `json:"build_extra_args,omitempty"`
	ComposeCmdWrapper        string              `json:"compose_cmd_wrapper,omitempty"`
	ComposeCmdWrapperInclude []string            `json:"compose_cmd_wrapper_include,omitempty"`
	IgnoreServices           []string            `json:"ignore_services,omitempty"`
	// Pointers to slices: nil leaves the field alone, an empty slice clears it.
	AutoUpdateSkipServices *[]string              `json:"auto_update_skip_services,omitempty"`
	ConfigFiles            *[]StackFileDependency `json:"config_files,omitempty"`
	SendAlerts             *bool                  `json:"send_alerts,omitempty"`
}

type Stack struct {
	ID     string      `json:"-"`
	Name   string      `json:"name"`
	Tags   []string    `json:"tags,omitempty"`
	Config StackConfig `json:"config"`
}

func (s *Stack) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID     json.RawMessage `json:"_id"`
		Name   string          `json:"name"`
		Tags   []string        `json:"tags,omitempty"`
		Config StackConfig     `json:"config"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	s.ID = parseMongoID(a.ID)
	s.Name = a.Name
	s.Tags = a.Tags
	s.Config = a.Config
	return nil
}

type CreateStackParams struct {
	Name   string      `json:"name"`
	Config StackConfig `json:"config,omitempty"`
}

type UpdateStackParams struct {
	ID     string      `json:"id"`
	Config StackConfig `json:"config"`
}

func (c *Client) CreateStack(ctx context.Context, params CreateStackParams) (*Stack, error) {
	var s Stack
	err := c.Write(ctx, "CreateStack", params, &s)
	return &s, err
}

func (c *Client) GetStack(ctx context.Context, idOrName string) (*Stack, error) {
	if idOrName == "" {
		return nil, &NotFoundError{Message: "stack identifier is empty"}
	}
	var s Stack
	err := c.Read(ctx, "GetStack", map[string]string{"stack": idOrName}, &s)
	return &s, err
}

func (c *Client) UpdateStack(ctx context.Context, params UpdateStackParams) (*Stack, error) {
	var s Stack
	err := c.Write(ctx, "UpdateStack", params, &s)
	return &s, err
}

func (c *Client) DeleteStack(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteStack", IDParam{ID: id}, nil)
}

// StackService represents a single service within a stack and its running state.
type StackService struct {
	Service   string          `json:"service"`
	Image     string          `json:"image"`
	Container json.RawMessage `json:"container"` // null when not running
}

// IsRunning returns true when the service has an active container.
func (s StackService) IsRunning() bool {
	return len(s.Container) > 0 && string(s.Container) != "null"
}

func (c *Client) ListStackServices(ctx context.Context, stackID string) ([]StackService, error) {
	var services []StackService
	err := c.Read(ctx, "ListStackServices", map[string]string{"stack": stackID}, &services)
	return services, err
}

func (c *Client) StartStack(ctx context.Context, stackID string) error {
	return c.Execute(ctx, "StartStack", map[string]string{"stack": stackID}, nil)
}

func (c *Client) StopStack(ctx context.Context, stackID string) error {
	return c.Execute(ctx, "StopStack", map[string]string{"stack": stackID}, nil)
}

func (c *Client) DeployStack(ctx context.Context, stackID string) error {
	return c.Execute(ctx, "DeployStack", map[string]string{"stack": stackID}, nil)
}

func (c *Client) DestroyStack(ctx context.Context, stackID string) error {
	return c.Execute(ctx, "DestroyStack", map[string]interface{}{
		"stack":          stackID,
		"services":       []string{},
		"remove_orphans": false,
	}, nil)
}

// ---- Deployment ----

// DeploymentImage is an adjacently-tagged enum serialized as:
//
//	{"type": "Image", "params": {"image": "nginx:latest"}}
//	{"type": "Build", "params": {"build_id": "..."}}
type DeploymentImage struct {
	Type   string                `json:"type"`
	Params DeploymentImageParams `json:"params"`
}

type DeploymentImageParams struct {
	Image   string `json:"image,omitempty"`
	BuildID string `json:"build_id,omitempty"`
}

type DeploymentConfig struct {
	ServerID             string          `json:"server_id,omitempty"`
	SwarmID              string          `json:"swarm_id,omitempty"`
	CustomName           *string         `json:"custom_name,omitempty"`
	Image                DeploymentImage `json:"image"`
	ImageRegistryAccount string          `json:"image_registry_account,omitempty"`
	SkipSecretInterp     *bool           `json:"skip_secret_interp,omitempty"`
	PollForUpdates       *bool           `json:"poll_for_updates,omitempty"`
	Links                []string        `json:"links,omitempty"`
	Network              string          `json:"network,omitempty"`
	RestartMode          string          `json:"restart,omitempty"`
	Replicas             *int            `json:"replicas,omitempty"`
	TerminationSignal    string          `json:"termination_signal,omitempty"`
	TerminationTimeout   *int            `json:"termination_timeout,omitempty"`
	TermSignalLabels     string          `json:"term_signal_labels,omitempty"`
	Ports                string          `json:"ports,omitempty"`
	Volumes              string          `json:"volumes,omitempty"`
	Environment          string          `json:"environment,omitempty"`
	Labels               string          `json:"labels,omitempty"`
	ExtraArgs            []string        `json:"extra_args,omitempty"`
	Command              string          `json:"command,omitempty"`
	SendAlerts           *bool           `json:"send_alerts,omitempty"`
	AutoUpdate           *bool           `json:"auto_update,omitempty"`
	Redeploy             *bool           `json:"redeploy_on_build,omitempty"`
}

type Deployment struct {
	ID     string           `json:"-"`
	Name   string           `json:"name"`
	Tags   []string         `json:"tags,omitempty"`
	Config DeploymentConfig `json:"config"`
}

func (d *Deployment) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID     json.RawMessage  `json:"_id"`
		Name   string           `json:"name"`
		Tags   []string         `json:"tags,omitempty"`
		Config DeploymentConfig `json:"config"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	d.ID = parseMongoID(a.ID)
	d.Name = a.Name
	d.Tags = a.Tags
	d.Config = a.Config
	return nil
}

type CreateDeploymentParams struct {
	Name   string           `json:"name"`
	Config DeploymentConfig `json:"config,omitempty"`
}

type UpdateDeploymentParams struct {
	ID     string           `json:"id"`
	Config DeploymentConfig `json:"config"`
}

func (c *Client) CreateDeployment(ctx context.Context, params CreateDeploymentParams) (*Deployment, error) {
	var d Deployment
	err := c.Write(ctx, "CreateDeployment", params, &d)
	return &d, err
}

func (c *Client) GetDeployment(ctx context.Context, idOrName string) (*Deployment, error) {
	var d Deployment
	err := c.Read(ctx, "GetDeployment", map[string]string{"deployment": idOrName}, &d)
	return &d, err
}

func (c *Client) UpdateDeployment(ctx context.Context, params UpdateDeploymentParams) (*Deployment, error) {
	var d Deployment
	err := c.Write(ctx, "UpdateDeployment", params, &d)
	return &d, err
}

func (c *Client) DeleteDeployment(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteDeployment", IDParam{ID: id}, nil)
}

func (c *Client) DeployDeployment(ctx context.Context, idOrName string) error {
	return c.Execute(ctx, "Deploy", map[string]string{"deployment": idOrName}, nil)
}

func (c *Client) DestroyDeployment(ctx context.Context, idOrName string) error {
	return c.Execute(ctx, "DestroyDeployment", map[string]string{"deployment": idOrName}, nil)
}

// ---- Build ----

// ImageRegistryConfig describes a registry to push a built image to.
type ImageRegistryConfig struct {
	Domain       string `json:"domain"`
	Account      string `json:"account"`
	Organization string `json:"organization"`
}

// BuildVersion is the build's semantic version. Komodo serializes it as an
// object and accepts either that object or a "major.minor.patch" string.
type BuildVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

func (v BuildVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// ParseBuildVersion parses a "major.minor.patch" string.
func ParseBuildVersion(s string) (*BuildVersion, error) {
	var v BuildVersion
	var extra string
	if n, _ := fmt.Sscanf(s, "%d.%d.%d%s", &v.Major, &v.Minor, &v.Patch, &extra); n != 3 || v.Major < 0 || v.Minor < 0 || v.Patch < 0 {
		return nil, fmt.Errorf("invalid version %q: expected major.minor.patch, e.g. 1.2.3", s)
	}
	return &v, nil
}

type BuildConfig struct {
	BuilderID            string                `json:"builder_id,omitempty"`
	Version              *BuildVersion         `json:"version,omitempty"`
	ImageName            string                `json:"image_name,omitempty"`
	ImageTag             string                `json:"image_tag,omitempty"`
	AutoIncrementVersion *bool                 `json:"auto_increment_version,omitempty"`
	IncludeLatestTag     *bool                 `json:"include_latest_tag,omitempty"`
	IncludeVersionTags   *bool                 `json:"include_version_tags,omitempty"`
	IncludeCommitTag     *bool                 `json:"include_commit_tag,omitempty"`
	Links                []string              `json:"links,omitempty"`
	LinkedRepo           string                `json:"linked_repo,omitempty"`
	GitProvider          string                `json:"git_provider,omitempty"`
	GitHTTPS             *bool                 `json:"git_https,omitempty"`
	GitAccount           string                `json:"git_account,omitempty"`
	Repo                 string                `json:"repo,omitempty"`
	Branch               string                `json:"branch,omitempty"`
	Commit               string                `json:"commit,omitempty"`
	WebhookEnabled       *bool                 `json:"webhook_enabled,omitempty"`
	WebhookSecret        string                `json:"webhook_secret,omitempty"`
	FilesOnHost          *bool                 `json:"files_on_host,omitempty"`
	BuildPath            string                `json:"build_path,omitempty"`
	DockerfilePath       string                `json:"dockerfile_path,omitempty"`
	Dockerfile           string                `json:"dockerfile,omitempty"`
	ImageRegistry        []ImageRegistryConfig `json:"image_registry,omitempty"`
	SkipSecretInterp     *bool                 `json:"skip_secret_interp,omitempty"`
	UseBuildx            *bool                 `json:"use_buildx,omitempty"`
	ExtraArgs            *[]string             `json:"extra_args,omitempty"`
	PreBuild             *SystemCommand        `json:"pre_build,omitempty"`
	BuildArgs            string                `json:"build_args,omitempty"`
	SecretArgs           *string               `json:"secret_args,omitempty"`
	Labels               string                `json:"labels,omitempty"`
}

type Build struct {
	ID     string      `json:"-"`
	Name   string      `json:"name"`
	Tags   []string    `json:"tags,omitempty"`
	Config BuildConfig `json:"config"`
}

func (b *Build) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID     json.RawMessage `json:"_id"`
		Name   string          `json:"name"`
		Tags   []string        `json:"tags,omitempty"`
		Config BuildConfig     `json:"config"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	b.ID = parseMongoID(a.ID)
	b.Name = a.Name
	b.Tags = a.Tags
	b.Config = a.Config
	return nil
}

type CreateBuildParams struct {
	Name   string      `json:"name"`
	Config BuildConfig `json:"config,omitempty"`
}

type UpdateBuildParams struct {
	ID     string      `json:"id"`
	Config BuildConfig `json:"config"`
}

func (c *Client) CreateBuild(ctx context.Context, params CreateBuildParams) (*Build, error) {
	var b Build
	err := c.Write(ctx, "CreateBuild", params, &b)
	return &b, err
}

func (c *Client) GetBuild(ctx context.Context, idOrName string) (*Build, error) {
	var b Build
	err := c.Read(ctx, "GetBuild", map[string]string{"build": idOrName}, &b)
	return &b, err
}

func (c *Client) UpdateBuild(ctx context.Context, params UpdateBuildParams) (*Build, error) {
	var b Build
	err := c.Write(ctx, "UpdateBuild", params, &b)
	return &b, err
}

func (c *Client) DeleteBuild(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteBuild", IDParam{ID: id}, nil)
}

// ---- Repo ----

type RepoConfig struct {
	ServerID         string         `json:"server_id,omitempty"`
	BuilderID        string         `json:"builder_id,omitempty"`
	GitProvider      string         `json:"git_provider,omitempty"`
	GitHTTPS         *bool          `json:"git_https,omitempty"`
	GitAccount       string         `json:"git_account,omitempty"`
	Repo             string         `json:"repo,omitempty"`
	Branch           string         `json:"branch,omitempty"`
	Commit           string         `json:"commit,omitempty"`
	Path             string         `json:"path,omitempty"`
	OnClone          *SystemCommand `json:"on_clone,omitempty"`
	OnPull           *SystemCommand `json:"on_pull,omitempty"`
	WebhookEnabled   *bool          `json:"webhook_enabled,omitempty"`
	WebhookSecret    string         `json:"webhook_secret,omitempty"`
	Links            []string       `json:"links,omitempty"`
	Environment      string         `json:"environment,omitempty"`
	EnvFilePath      string         `json:"env_file_path,omitempty"`
	SkipSecretInterp *bool          `json:"skip_secret_interp,omitempty"`
}

type Repo struct {
	ID     string     `json:"-"`
	Name   string     `json:"name"`
	Tags   []string   `json:"tags,omitempty"`
	Config RepoConfig `json:"config"`
}

func (r *Repo) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID     json.RawMessage `json:"_id"`
		Name   string          `json:"name"`
		Tags   []string        `json:"tags,omitempty"`
		Config RepoConfig      `json:"config"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	r.ID = parseMongoID(a.ID)
	r.Name = a.Name
	r.Tags = a.Tags
	r.Config = a.Config
	return nil
}

type CreateRepoParams struct {
	Name   string     `json:"name"`
	Config RepoConfig `json:"config,omitempty"`
}

type UpdateRepoParams struct {
	ID     string     `json:"id"`
	Config RepoConfig `json:"config"`
}

func (c *Client) CreateRepo(ctx context.Context, params CreateRepoParams) (*Repo, error) {
	var r Repo
	err := c.Write(ctx, "CreateRepo", params, &r)
	return &r, err
}

func (c *Client) GetRepo(ctx context.Context, idOrName string) (*Repo, error) {
	var r Repo
	err := c.Read(ctx, "GetRepo", map[string]string{"repo": idOrName}, &r)
	return &r, err
}

func (c *Client) UpdateRepo(ctx context.Context, params UpdateRepoParams) (*Repo, error) {
	var r Repo
	err := c.Write(ctx, "UpdateRepo", params, &r)
	return &r, err
}

func (c *Client) DeleteRepo(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteRepo", IDParam{ID: id}, nil)
}

// ---- Tag ----

type Tag struct {
	ID    string `json:"-"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

func (t *Tag) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID    json.RawMessage `json:"_id"`
		Name  string          `json:"name"`
		Color string          `json:"color,omitempty"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	t.ID = parseMongoID(a.ID)
	t.Name = a.Name
	t.Color = a.Color
	return nil
}

type CreateTagParams struct {
	Name string `json:"name"`
}

func (c *Client) CreateTag(ctx context.Context, params CreateTagParams) (*Tag, error) {
	var t Tag
	err := c.Write(ctx, "CreateTag", params, &t)
	return &t, err
}

func (c *Client) GetTag(ctx context.Context, idOrName string) (*Tag, error) {
	// Tags are read via ListTags — filter client-side.
	tags, err := c.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tags {
		if t.ID == idOrName || t.Name == idOrName {
			return &t, nil
		}
	}
	return nil, &NotFoundError{Message: fmt.Sprintf("tag not found: %s", idOrName)}
}

func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	var tags []Tag
	err := c.Read(ctx, "ListTags", struct{}{}, &tags)
	return tags, err
}

func (c *Client) RenameTag(ctx context.Context, id string, name string) (*Tag, error) {
	var t Tag
	err := c.Write(ctx, "RenameTag", map[string]string{"id": id, "name": name}, &t)
	return &t, err
}

func (c *Client) UpdateTagColor(ctx context.Context, idOrName string, color string) (*Tag, error) {
	var t Tag
	err := c.Write(ctx, "UpdateTagColor", map[string]string{"tag": idOrName, "color": color}, &t)
	return &t, err
}

func (c *Client) DeleteTag(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteTag", IDParam{ID: id}, nil)
}

// ---- ResourceMeta ----

type ResourceTarget struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type UpdateResourceMetaParams struct {
	Target ResourceTarget `json:"target"`
	Tags   []string       `json:"tags"`
}

func (c *Client) UpdateResourceMeta(ctx context.Context, targetType, id string, tags []string) error {
	return c.Write(ctx, "UpdateResourceMeta", UpdateResourceMetaParams{
		Target: ResourceTarget{Type: targetType, ID: id},
		Tags:   tags,
	}, nil)
}

// ResourceMeta is the description and template flag every Komodo resource
// carries alongside its tags.
type ResourceMeta struct {
	Description string `json:"description"`
	Template    bool   `json:"template"`
}

// UpdateResourceMetaFields sets a resource's description and/or template
// flag. A nil argument leaves that field as it is. Tags are not touched.
func (c *Client) UpdateResourceMetaFields(ctx context.Context, targetType, id string, description *string, template *bool) error {
	params := struct {
		Target      ResourceTarget `json:"target"`
		Description *string        `json:"description,omitempty"`
		Template    *bool          `json:"template,omitempty"`
	}{Target: ResourceTarget{Type: targetType, ID: id}, Description: description, Template: template}
	return c.Write(ctx, "UpdateResourceMeta", params, nil)
}

// GetResourceMeta reads the description and template flag of a resource of
// the given type ("Server", "Stack", ...) by ID or name.
func (c *Client) GetResourceMeta(ctx context.Context, targetType, idOrName string) (*ResourceMeta, error) {
	var m ResourceMeta
	key := strings.ToLower(targetType)
	err := c.Read(ctx, "Get"+targetType, map[string]string{key: idOrName}, &m)
	return &m, err
}

// ---- Builder ----

// BuilderConfig is an adjacently-tagged enum on the wire:
//
//	{"type": "Server", "params": {"server_id": "..."}}
//	{"type": "Url",    "params": {...}}
//	{"type": "Aws",    "params": {...}}
//
// The provider currently exposes only the "Server" variant. Custom Marshal /
// Unmarshal keep the Go-side shape flat so callers can use Config.ServerID.
type BuilderConfig struct {
	ServerID string `json:"-"`
}

func (c BuilderConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   string `json:"type"`
		Params struct {
			ServerID string `json:"server_id,omitempty"`
		} `json:"params"`
	}{
		Type: "Server",
		Params: struct {
			ServerID string `json:"server_id,omitempty"`
		}{ServerID: c.ServerID},
	})
}

func (c *BuilderConfig) UnmarshalJSON(data []byte) error {
	// Newer Komodo returns a Server builder's servers as a `server_ids` list
	// (builds overflow to later entries); older versions return a single
	// `server_id`. Both are accepted on write. Read whichever is present.
	var wrapper struct {
		Type   string `json:"type"`
		Params struct {
			ServerID  string   `json:"server_id,omitempty"`
			ServerIDs []string `json:"server_ids,omitempty"`
		} `json:"params"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		// Fall back to flat decode for backwards compatibility.
		var flat struct {
			ServerID string `json:"server_id,omitempty"`
		}
		if err2 := json.Unmarshal(data, &flat); err2 != nil {
			return err
		}
		c.ServerID = flat.ServerID
		return nil
	}
	c.ServerID = wrapper.Params.ServerID
	if c.ServerID == "" && len(wrapper.Params.ServerIDs) > 0 {
		c.ServerID = wrapper.Params.ServerIDs[0]
	}
	return nil
}

type Builder struct {
	ID     string        `json:"-"`
	Name   string        `json:"name"`
	Tags   []string      `json:"tags,omitempty"`
	Config BuilderConfig `json:"config"`
}

func (b *Builder) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID     json.RawMessage `json:"_id"`
		Name   string          `json:"name"`
		Tags   []string        `json:"tags,omitempty"`
		Config BuilderConfig   `json:"config"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	b.ID = parseMongoID(a.ID)
	b.Name = a.Name
	b.Tags = a.Tags
	b.Config = a.Config
	return nil
}

type CreateBuilderParams struct {
	Name   string        `json:"name"`
	Config BuilderConfig `json:"config,omitempty"`
}

type UpdateBuilderParams struct {
	ID     string        `json:"id"`
	Config BuilderConfig `json:"config"`
}

func (c *Client) CreateBuilder(ctx context.Context, params CreateBuilderParams) (*Builder, error) {
	var b Builder
	err := c.Write(ctx, "CreateBuilder", params, &b)
	return &b, err
}

func (c *Client) GetBuilder(ctx context.Context, idOrName string) (*Builder, error) {
	var b Builder
	err := c.Read(ctx, "GetBuilder", map[string]string{"builder": idOrName}, &b)
	return &b, err
}

func (c *Client) UpdateBuilder(ctx context.Context, params UpdateBuilderParams) (*Builder, error) {
	var b Builder
	err := c.Write(ctx, "UpdateBuilder", params, &b)
	return &b, err
}

func (c *Client) DeleteBuilder(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteBuilder", IDParam{ID: id}, nil)
}

// ---- User ----

type User struct {
	ID                      string `json:"-"`
	Username                string `json:"username"`
	Enabled                 bool   `json:"enabled"`
	Admin                   bool   `json:"admin"`
	SuperAdmin              bool   `json:"super_admin"`
	CreateServerPermissions bool   `json:"create_server_permissions"`
	CreateBuildPermissions  bool   `json:"create_build_permissions"`
}

func (u *User) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID                      json.RawMessage `json:"_id"`
		Username                string          `json:"username"`
		Enabled                 bool            `json:"enabled"`
		Admin                   bool            `json:"admin"`
		SuperAdmin              bool            `json:"super_admin"`
		CreateServerPermissions bool            `json:"create_server_permissions"`
		CreateBuildPermissions  bool            `json:"create_build_permissions"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	u.ID = parseMongoID(a.ID)
	u.Username = a.Username
	u.Enabled = a.Enabled
	u.Admin = a.Admin
	u.SuperAdmin = a.SuperAdmin
	u.CreateServerPermissions = a.CreateServerPermissions
	u.CreateBuildPermissions = a.CreateBuildPermissions
	return nil
}

func (c *Client) CreateLocalUser(ctx context.Context, username, password string) (*User, error) {
	var u User
	err := c.Write(ctx, "CreateLocalUser", map[string]string{
		"username": username,
		"password": password,
	}, &u)
	return &u, err
}

func (c *Client) CreateServiceUser(ctx context.Context, username, description string) (*User, error) {
	var u User
	err := c.Write(ctx, "CreateServiceUser", map[string]string{
		"username":    username,
		"description": description,
	}, &u)
	return &u, err
}

func (c *Client) FindUser(ctx context.Context, idOrName string) (*User, error) {
	var u User
	err := c.Read(ctx, "FindUser", map[string]string{"user": idOrName}, &u)
	return &u, err
}

func (c *Client) UpdateUserBasePermissions(ctx context.Context, userID string, enabled, createServer, createBuild bool) error {
	// Note the v2 request-side field renames: `create_servers` / `create_builds`.
	// The corresponding response fields on the User struct are still
	// `create_server_permissions` / `create_build_permissions`.
	return c.Write(ctx, "UpdateUserBasePermissions", map[string]interface{}{
		"user_id":        userID,
		"enabled":        enabled,
		"create_servers": createServer,
		"create_builds":  createBuild,
	}, nil)
}

func (c *Client) UpdateUserAdmin(ctx context.Context, userID string, admin bool) error {
	return c.Write(ctx, "UpdateUserAdmin", map[string]interface{}{
		"user_id": userID,
		"admin":   admin,
	}, nil)
}

func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	return c.Write(ctx, "DeleteUser", map[string]string{"user": userID}, nil)
}

// ---- API Keys ----

type ApiKeyResponse struct {
	Key    string `json:"key"`
	Secret string `json:"secret"`
}

// CreateApiKeyForServiceUser creates a key. expires is a unix timestamp in
// milliseconds; 0 means the key never expires.
func (c *Client) CreateApiKeyForServiceUser(ctx context.Context, userID, name string, expires int64) (*ApiKeyResponse, error) {
	var r ApiKeyResponse
	err := c.Write(ctx, "CreateApiKeyForServiceUser", map[string]interface{}{
		"user_id": userID,
		"name":    name,
		"expires": expires,
	}, &r)
	return &r, err
}

func (c *Client) DeleteApiKeyForServiceUser(ctx context.Context, key string) error {
	return c.Write(ctx, "DeleteApiKeyForServiceUser", map[string]string{"key": key}, nil)
}

// ---- Onboarding Keys ----

// OnboardingKey is a server bootstrap credential. Periphery presents the
// matching private key on first contact and Core registers a Server using
// this key's settings.
type OnboardingKey struct {
	ID            string   `json:"-"`
	Name          string   `json:"name"`
	PublicKey     string   `json:"public_key"`
	Enabled       bool     `json:"enabled"`
	Expires       int64    `json:"expires"`
	Privileged    bool     `json:"privileged"`
	CopyServer    string   `json:"copy_server,omitempty"`
	CreateBuilder bool     `json:"create_builder"`
	Tags          []string `json:"tags,omitempty"`
}

func (k *OnboardingKey) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID            json.RawMessage `json:"_id"`
		Name          string          `json:"name"`
		PublicKey     string          `json:"public_key"`
		Enabled       bool            `json:"enabled"`
		Expires       int64           `json:"expires"`
		Privileged    bool            `json:"privileged"`
		CopyServer    string          `json:"copy_server,omitempty"`
		CreateBuilder bool            `json:"create_builder"`
		Tags          []string        `json:"tags,omitempty"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	k.ID = parseMongoID(a.ID)
	k.Name = a.Name
	k.PublicKey = a.PublicKey
	k.Enabled = a.Enabled
	k.Expires = a.Expires
	k.Privileged = a.Privileged
	k.CopyServer = a.CopyServer
	k.CreateBuilder = a.CreateBuilder
	k.Tags = a.Tags
	return nil
}

type CreateOnboardingKeyParams struct {
	Name          string   `json:"name"`
	Expires       int64    `json:"expires"`
	PrivateKey    string   `json:"private_key,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Privileged    bool     `json:"privileged"`
	CopyServer    string   `json:"copy_server,omitempty"`
	CreateBuilder bool     `json:"create_builder"`
}

type CreateOnboardingKeyResponse struct {
	PrivateKey string        `json:"private_key"`
	Created    OnboardingKey `json:"created"`
}

type UpdateOnboardingKeyParams struct {
	PublicKey     string    `json:"public_key"`
	Name          *string   `json:"name,omitempty"`
	Enabled       *bool     `json:"enabled,omitempty"`
	Expires       *int64    `json:"expires,omitempty"`
	Tags          *[]string `json:"tags,omitempty"`
	Privileged    *bool     `json:"privileged,omitempty"`
	CopyServer    *string   `json:"copy_server,omitempty"`
	CreateBuilder *bool     `json:"create_builder,omitempty"`
}

func (c *Client) CreateOnboardingKey(ctx context.Context, params CreateOnboardingKeyParams) (*CreateOnboardingKeyResponse, error) {
	var r CreateOnboardingKeyResponse
	err := c.Write(ctx, "CreateOnboardingKey", params, &r)
	return &r, err
}

func (c *Client) UpdateOnboardingKey(ctx context.Context, params UpdateOnboardingKeyParams) (*OnboardingKey, error) {
	var k OnboardingKey
	err := c.Write(ctx, "UpdateOnboardingKey", params, &k)
	return &k, err
}

func (c *Client) DeleteOnboardingKey(ctx context.Context, publicKey string) error {
	return c.Write(ctx, "DeleteOnboardingKey", map[string]string{"public_key": publicKey}, nil)
}

func (c *Client) ListOnboardingKeys(ctx context.Context) ([]OnboardingKey, error) {
	var keys []OnboardingKey
	err := c.Read(ctx, "ListOnboardingKeys", struct{}{}, &keys)
	return keys, err
}

// FindOnboardingKey returns the first onboarding key matching the given public_key.
// Komodo has no GetOnboardingKey by ID; reads always go through ListOnboardingKeys.
func (c *Client) FindOnboardingKey(ctx context.Context, publicKey string) (*OnboardingKey, error) {
	keys, err := c.ListOnboardingKeys(ctx)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].PublicKey == publicKey {
			return &keys[i], nil
		}
	}
	return nil, &NotFoundError{Message: fmt.Sprintf("onboarding key with public_key %q not found", publicKey)}
}

// ---- Git Provider Accounts ----

type GitProviderAccount struct {
	ID       string `json:"id"`
	Domain   string `json:"domain"`
	Https    bool   `json:"https"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

func (g *GitProviderAccount) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID       json.RawMessage `json:"_id"`
		Domain   string          `json:"domain"`
		Https    bool            `json:"https"`
		Username string          `json:"username"`
		Token    string          `json:"token"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	g.ID = parseMongoID(a.ID)
	g.Domain = a.Domain
	g.Https = a.Https
	g.Username = a.Username
	g.Token = a.Token
	return nil
}

func (c *Client) CreateGitProviderAccount(ctx context.Context, domain, username, token string, https bool) (*GitProviderAccount, error) {
	var g GitProviderAccount
	err := c.Write(ctx, "CreateGitProviderAccount", map[string]interface{}{
		"account": map[string]interface{}{
			"domain":   domain,
			"username": username,
			"token":    token,
			"https":    https,
		},
	}, &g)
	return &g, err
}

func (c *Client) UpdateGitProviderAccount(ctx context.Context, id, domain, username, token string, https bool) (*GitProviderAccount, error) {
	var g GitProviderAccount
	err := c.Write(ctx, "UpdateGitProviderAccount", map[string]interface{}{
		"id": id,
		"account": map[string]interface{}{
			"domain":   domain,
			"username": username,
			"token":    token,
			"https":    https,
		},
	}, &g)
	return &g, err
}

func (c *Client) GetGitProviderAccount(ctx context.Context, id string) (*GitProviderAccount, error) {
	var g GitProviderAccount
	err := c.Read(ctx, "GetGitProviderAccount", map[string]string{"id": id}, &g)
	if err != nil {
		return nil, err
	}
	if g.ID == "" {
		return nil, &NotFoundError{Message: fmt.Sprintf("git provider account %q not found", id)}
	}
	return &g, nil
}

func (c *Client) DeleteGitProviderAccount(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteGitProviderAccount", map[string]string{"id": id}, nil)
}

// ---- Docker Registry Accounts ----

type DockerRegistryAccount struct {
	ID       string `json:"id"`
	Domain   string `json:"domain"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

func (d *DockerRegistryAccount) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID       json.RawMessage `json:"_id"`
		Domain   string          `json:"domain"`
		Username string          `json:"username"`
		Token    string          `json:"token"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	d.ID = parseMongoID(a.ID)
	d.Domain = a.Domain
	d.Username = a.Username
	d.Token = a.Token
	return nil
}

func (c *Client) CreateDockerRegistryAccount(ctx context.Context, domain, username, token string) (*DockerRegistryAccount, error) {
	var d DockerRegistryAccount
	err := c.Write(ctx, "CreateDockerRegistryAccount", map[string]interface{}{
		"account": map[string]interface{}{
			"domain":   domain,
			"username": username,
			"token":    token,
		},
	}, &d)
	return &d, err
}

func (c *Client) UpdateDockerRegistryAccount(ctx context.Context, id, domain, username, token string) (*DockerRegistryAccount, error) {
	var d DockerRegistryAccount
	err := c.Write(ctx, "UpdateDockerRegistryAccount", map[string]interface{}{
		"id": id,
		"account": map[string]interface{}{
			"domain":   domain,
			"username": username,
			"token":    token,
		},
	}, &d)
	return &d, err
}

func (c *Client) GetDockerRegistryAccount(ctx context.Context, id string) (*DockerRegistryAccount, error) {
	var d DockerRegistryAccount
	err := c.Read(ctx, "GetDockerRegistryAccount", map[string]string{"id": id}, &d)
	if err != nil {
		return nil, err
	}
	if d.ID == "" {
		return nil, &NotFoundError{Message: fmt.Sprintf("docker registry account %q not found", id)}
	}
	return &d, nil
}

func (c *Client) DeleteDockerRegistryAccount(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteDockerRegistryAccount", map[string]string{"id": id}, nil)
}

// Helper to get a bool pointer
func BoolPtr(b bool) *bool {
	return &b
}

func Float64Ptr(f float64) *float64 {
	return &f
}

func IntPtr(i int) *int {
	return &i
}

func StringPtr(s string) *string {
	return &s
}

// ---- Variable ----

// Variable is a global key/value that Komodo interpolates wherever
// `[[NAME]]` appears in resource config. Variables are keyed by name and have
// no separate ID.
type Variable struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value"`
	IsSecret    bool   `json:"is_secret"`
}

func (c *Client) CreateVariable(ctx context.Context, v Variable) (*Variable, error) {
	var out Variable
	err := c.Write(ctx, "CreateVariable", map[string]interface{}{
		"name":        v.Name,
		"value":       v.Value,
		"description": v.Description,
		"is_secret":   v.IsSecret,
	}, &out)
	return &out, err
}

func (c *Client) ListVariables(ctx context.Context) ([]Variable, error) {
	var vars []Variable
	err := c.Read(ctx, "ListVariables", struct{}{}, &vars)
	return vars, err
}

// GetVariable looks the variable up in ListVariables rather than calling
// GetVariable: Komodo answers a missing variable with a generic HTTP 500,
// which cannot be told apart from a real server error (and would be retried).
func (c *Client) GetVariable(ctx context.Context, name string) (*Variable, error) {
	vars, err := c.ListVariables(ctx)
	if err != nil {
		return nil, err
	}
	for i := range vars {
		if vars[i].Name == name {
			return &vars[i], nil
		}
	}
	return nil, &NotFoundError{Message: fmt.Sprintf("variable not found: %s", name)}
}

func (c *Client) UpdateVariableValue(ctx context.Context, name, value string) error {
	return c.Write(ctx, "UpdateVariableValue", map[string]string{"name": name, "value": value}, nil)
}

func (c *Client) UpdateVariableDescription(ctx context.Context, name, description string) error {
	return c.Write(ctx, "UpdateVariableDescription", map[string]string{"name": name, "description": description}, nil)
}

func (c *Client) UpdateVariableIsSecret(ctx context.Context, name string, isSecret bool) error {
	return c.Write(ctx, "UpdateVariableIsSecret", map[string]interface{}{"name": name, "is_secret": isSecret}, nil)
}

func (c *Client) DeleteVariable(ctx context.Context, name string) error {
	return c.Write(ctx, "DeleteVariable", map[string]string{"name": name}, nil)
}

// ---- Alerter ----

// AlerterEndpoint is an adjacently-tagged enum on the wire:
//
//	{"type": "Slack", "params": {"url": "..."}}
//
// Type is one of Custom, Slack, Discord, Ntfy, Pushover. Email only applies
// to Ntfy.
type AlerterEndpoint struct {
	Type  string
	URL   string
	Email string
}

type alerterEndpointParams struct {
	URL   string  `json:"url"`
	Email *string `json:"email,omitempty"`
}

type alerterEndpointWire struct {
	Type   string                `json:"type"`
	Params alerterEndpointParams `json:"params"`
}

func (e AlerterEndpoint) MarshalJSON() ([]byte, error) {
	w := alerterEndpointWire{Type: e.Type, Params: alerterEndpointParams{URL: e.URL}}
	if e.Type == "Ntfy" && e.Email != "" {
		w.Params.Email = &e.Email
	}
	return json.Marshal(w)
}

func (e *AlerterEndpoint) UnmarshalJSON(data []byte) error {
	var w alerterEndpointWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	e.Type = w.Type
	e.URL = w.Params.URL
	e.Email = ""
	if w.Params.Email != nil {
		e.Email = *w.Params.Email
	}
	return nil
}

// AlerterConfig is sent in full on create and update. The slices carry no
// omitempty so that an empty list clears the filter in Komodo.
type AlerterConfig struct {
	Enabled         bool             `json:"enabled"`
	Endpoint        AlerterEndpoint  `json:"endpoint"`
	AlertTypes      []string         `json:"alert_types"`
	Resources       []ResourceTarget `json:"resources"`
	ExceptResources []ResourceTarget `json:"except_resources"`
}

// normalize replaces nil slices with empty ones so they marshal as [] and
// not null.
func (c *AlerterConfig) normalize() {
	if c.AlertTypes == nil {
		c.AlertTypes = []string{}
	}
	if c.Resources == nil {
		c.Resources = []ResourceTarget{}
	}
	if c.ExceptResources == nil {
		c.ExceptResources = []ResourceTarget{}
	}
}

type Alerter struct {
	ID     string        `json:"-"`
	Name   string        `json:"name"`
	Tags   []string      `json:"tags,omitempty"`
	Config AlerterConfig `json:"config"`
}

func (a *Alerter) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID     json.RawMessage `json:"_id"`
		Name   string          `json:"name"`
		Tags   []string        `json:"tags,omitempty"`
		Config AlerterConfig   `json:"config"`
	}
	var al alias
	if err := json.Unmarshal(data, &al); err != nil {
		return err
	}
	a.ID = parseMongoID(al.ID)
	a.Name = al.Name
	a.Tags = al.Tags
	a.Config = al.Config
	return nil
}

func (c *Client) CreateAlerter(ctx context.Context, name string, config AlerterConfig) (*Alerter, error) {
	config.normalize()
	var a Alerter
	err := c.Write(ctx, "CreateAlerter", map[string]interface{}{"name": name, "config": config}, &a)
	return &a, err
}

func (c *Client) GetAlerter(ctx context.Context, idOrName string) (*Alerter, error) {
	if idOrName == "" {
		return nil, &NotFoundError{Message: "alerter identifier is empty"}
	}
	var a Alerter
	err := c.Read(ctx, "GetAlerter", map[string]string{"alerter": idOrName}, &a)
	return &a, err
}

func (c *Client) UpdateAlerter(ctx context.Context, id string, config AlerterConfig) (*Alerter, error) {
	config.normalize()
	var a Alerter
	err := c.Write(ctx, "UpdateAlerter", map[string]interface{}{"id": id, "config": config}, &a)
	return &a, err
}

func (c *Client) RenameAlerter(ctx context.Context, id, name string) error {
	return c.Write(ctx, "RenameAlerter", map[string]string{"id": id, "name": name}, nil)
}

func (c *Client) DeleteAlerter(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteAlerter", IDParam{ID: id}, nil)
}

// ---- Procedure ----

// Execution is an adjacently-tagged enum on the wire:
//
//	{"type": "DeployStack", "params": {"stack": "..."}}
//
// Params is kept as raw JSON because every execution type has its own shape.
type Execution struct {
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

type EnabledExecution struct {
	Execution Execution `json:"execution"`
	Enabled   bool      `json:"enabled"`
}

// ProcedureStage is a group of executions that run in parallel. Stages run
// one after another.
type ProcedureStage struct {
	Name       string             `json:"name"`
	Enabled    bool               `json:"enabled"`
	Executions []EnabledExecution `json:"executions"`
}

// ProcedureConfig is sent in full on create and update.
type ProcedureConfig struct {
	Stages           []ProcedureStage `json:"stages"`
	ScheduleFormat   string           `json:"schedule_format"`
	Schedule         string           `json:"schedule"`
	ScheduleEnabled  bool             `json:"schedule_enabled"`
	ScheduleTimezone string           `json:"schedule_timezone"`
	ScheduleAlert    bool             `json:"schedule_alert"`
	FailureAlert     bool             `json:"failure_alert"`
	WebhookEnabled   bool             `json:"webhook_enabled"`
	WebhookSecret    string           `json:"webhook_secret"`
}

// normalize makes nil slices and empty params marshal as [] and {}.
func (c *ProcedureConfig) normalize() {
	if c.Stages == nil {
		c.Stages = []ProcedureStage{}
	}
	for i := range c.Stages {
		if c.Stages[i].Executions == nil {
			c.Stages[i].Executions = []EnabledExecution{}
		}
		for j := range c.Stages[i].Executions {
			if len(c.Stages[i].Executions[j].Execution.Params) == 0 {
				c.Stages[i].Executions[j].Execution.Params = json.RawMessage(`{}`)
			}
		}
	}
}

// CanonicalStagesJSON renders stages as JSON with object keys sorted, so two
// values can be compared regardless of key order or whitespace in params.
func CanonicalStagesJSON(stages []ProcedureStage) ([]byte, error) {
	cfg := ProcedureConfig{Stages: stages}
	cfg.normalize()
	raw, err := json.Marshal(cfg.Stages)
	if err != nil {
		return nil, err
	}
	var generic interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic)
}

type Procedure struct {
	ID     string          `json:"-"`
	Name   string          `json:"name"`
	Tags   []string        `json:"tags,omitempty"`
	Config ProcedureConfig `json:"config"`
}

func (p *Procedure) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID     json.RawMessage `json:"_id"`
		Name   string          `json:"name"`
		Tags   []string        `json:"tags,omitempty"`
		Config ProcedureConfig `json:"config"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	p.ID = parseMongoID(a.ID)
	p.Name = a.Name
	p.Tags = a.Tags
	p.Config = a.Config
	return nil
}

func (c *Client) CreateProcedure(ctx context.Context, name string, config ProcedureConfig) (*Procedure, error) {
	config.normalize()
	var p Procedure
	err := c.Write(ctx, "CreateProcedure", map[string]interface{}{"name": name, "config": config}, &p)
	return &p, err
}

func (c *Client) GetProcedure(ctx context.Context, idOrName string) (*Procedure, error) {
	if idOrName == "" {
		return nil, &NotFoundError{Message: "procedure identifier is empty"}
	}
	var p Procedure
	err := c.Read(ctx, "GetProcedure", map[string]string{"procedure": idOrName}, &p)
	return &p, err
}

func (c *Client) UpdateProcedure(ctx context.Context, id string, config ProcedureConfig) (*Procedure, error) {
	config.normalize()
	var p Procedure
	err := c.Write(ctx, "UpdateProcedure", map[string]interface{}{"id": id, "config": config}, &p)
	return &p, err
}

func (c *Client) RenameProcedure(ctx context.Context, id, name string) error {
	return c.Write(ctx, "RenameProcedure", map[string]string{"id": id, "name": name}, nil)
}

func (c *Client) DeleteProcedure(ctx context.Context, id string) error {
	return c.Write(ctx, "DeleteProcedure", IDParam{ID: id}, nil)
}

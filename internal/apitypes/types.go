// Package apitypes is the CLI's own view of the naru REST wire contract: the
// request/response shapes it sends and receives. It is deliberately independent
// of the server's internal types — the CLI owns its structures and never imports
// or fetches the server's schema. The server-side X-Naru-Client-Schema drift
// tripwire (SchemaVersion below) flags if the two ever fall out of sync.
package apitypes

// SchemaVersion is the CLI's declared contract version, sent as
// X-Naru-Client-Schema. Bump it when these wire shapes change to match a server
// contract change; the server logs a Warning if it differs from its own.
const SchemaVersion = "1"

// CodeAppNotInstall is the one server error code the CLI branches on (to render
// the GitHub App install hint). Other branching is by HTTP status.
const CodeAppNotInstall = "github_app_not_installed"

// --- error envelope ---

// ErrorEnvelope wraps every error response: {"error": {...}}.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is the machine-readable error.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Details any    `json:"details,omitempty"`
}

// --- project-spec shapes (app/addon specs round-trip via get/apply) ---

// AppSpec is one application.
type AppSpec struct {
	Name      string         `json:"name"`
	Git       GitSpec        `json:"git"`
	Replicas  *int           `json:"replicas,omitempty"`
	Resources *ResourceSpec  `json:"resources,omitempty"`
	Rollout   *RolloutSpec   `json:"rollout,omitempty"`
	Endpoints []EndpointSpec `json:"endpoints,omitempty"`
}

// GitSpec is the source repository. type is always "github"; hash is CI-owned.
type GitSpec struct {
	Type   string `json:"type"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Hash   string `json:"hash,omitempty"`
}

// ResourceSpec mirrors requests/limits.
type ResourceSpec struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

// RolloutSpec controls the Argo Rollout strategy.
type RolloutSpec struct {
	Strategy       string        `json:"strategy,omitempty"`
	Surge          *IntOrPercent `json:"surge,omitempty"`
	Unavailable    *IntOrPercent `json:"unavailable,omitempty"`
	Promote        string        `json:"promote,omitempty"`
	ScaleDownDelay *int          `json:"scaleDownDelay,omitempty"`
	Steps          []RolloutStep `json:"steps,omitempty"`
}

// RolloutStep is one canary step. pause is seconds; 0 = wait for manual promote.
type RolloutStep struct {
	Weight int  `json:"weight"`
	Pause  *int `json:"pause,omitempty"`
}

// EndpointSpec is one exposed port.
type EndpointSpec struct {
	Port   int      `json:"port"`
	Name   string   `json:"name,omitempty"`
	Routes []string `json:"routes,omitempty"`
}

// AddonSpec is one addon.
type AddonSpec struct {
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	Version   string        `json:"version"`
	Port      int           `json:"port"`
	Size      string        `json:"size"`
	Resources *ResourceSpec `json:"resources,omitempty"`
}

// --- auth / meta ---

// AuthConfig is returned by GET /v1/auth/config.
type AuthConfig struct {
	ClientID string `json:"clientID"`
	AppSlug  string `json:"appSlug"`
}

// ExchangeRequest is the body of POST /v1/auth/exchange.
type ExchangeRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirectUri,omitempty"`
}

// AuthResponse is returned by POST /v1/auth/exchange.
type AuthResponse struct {
	Token     string `json:"token"`
	Username  string `json:"username"`
	GithubID  int64  `json:"githubID"`
	ExpiresAt string `json:"expiresAt"`
}

// MeResponse is returned by GET /v1/auth/me.
type MeResponse struct {
	GithubID  int64  `json:"githubID"`
	Username  string `json:"username"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// SchemaResponse is returned by GET /v1/schema (shown by `naru schema`).
type SchemaResponse struct {
	SchemaVersion string `json:"schemaVersion"`
	JSONSchema    any    `json:"jsonSchema"`
}

// --- projects ---

// ProjectSummary is a lightweight project listing entry.
type ProjectSummary struct {
	Name       string `json:"name"`
	AppCount   int    `json:"appCount"`
	AddonCount int    `json:"addonCount"`
}

// ProjectCreateRequest is the body of POST /v1/projects.
type ProjectCreateRequest struct {
	Name string `json:"name"`
}

// Project is the full project view.
type Project struct {
	Name         string      `json:"name"`
	Applications []AppSpec   `json:"applications"`
	Addons       []AddonSpec `json:"addons"`
}

// --- app/addon requests ---

// AppCreateRequest is the body of POST /v1/projects/{p}/apps.
type AppCreateRequest struct {
	Name      string         `json:"name"`
	Git       GitSpec        `json:"git"`
	Replicas  *int           `json:"replicas,omitempty"`
	Resources *ResourceSpec  `json:"resources,omitempty"`
	Rollout   *RolloutSpec   `json:"rollout,omitempty"`
	Endpoints []EndpointSpec `json:"endpoints,omitempty"`
}

// AppUpdateRequest is the body of PUT/PATCH /v1/projects/{p}/apps/{a}.
type AppUpdateRequest struct {
	Git       *GitSpec       `json:"git,omitempty"`
	Replicas  *int           `json:"replicas,omitempty"`
	Resources *ResourceSpec  `json:"resources,omitempty"`
	Rollout   *RolloutSpec   `json:"rollout,omitempty"`
	Endpoints []EndpointSpec `json:"endpoints,omitempty"`
}

// AddonCreateRequest is the body of POST /v1/projects/{p}/addons.
type AddonCreateRequest struct {
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	Version   string        `json:"version"`
	Port      *int          `json:"port,omitempty"`
	Size      string        `json:"size"`
	Resources *ResourceSpec `json:"resources,omitempty"`
}

// ConnectionInfo is the addon connection descriptor (passwordless).
type ConnectionInfo struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
}

// --- members ---

// MemberInfo is one project owner.
type MemberInfo struct {
	GithubID int64  `json:"githubID"`
	Username string `json:"username"`
}

// AddMemberRequest is the body of POST /v1/projects/{p}/members.
type AddMemberRequest struct {
	Username string `json:"username"`
}

// MembersResponse lists a project's owners.
type MembersResponse struct {
	Owners []MemberInfo `json:"owners"`
}

// --- secrets ---

// SecretVars is the body of PUT/PATCH secrets.
type SecretVars struct {
	Vars map[string]string `json:"vars"`
}

// SecretKeys is returned by GET secrets — keys only.
type SecretKeys struct {
	Keys []string `json:"keys"`
}

// --- ops ---

// StatusInfo summarizes an app's Rollout + pods (or an addon's StatefulSet).
type StatusInfo struct {
	Phase     string    `json:"phase"`
	Desired   int       `json:"desired"`
	Ready     int       `json:"ready"`
	Updated   int       `json:"updated"`
	Available int       `json:"available"`
	Revision  string    `json:"revision,omitempty"`
	Image     string    `json:"image,omitempty"`
	Pods      []PodInfo `json:"pods"`
}

// PodInfo is one pod's condensed status.
type PodInfo struct {
	Name                  string `json:"name"`
	Phase                 string `json:"phase"`
	Ready                 bool   `json:"ready"`
	Restarts              int    `json:"restarts"`
	Reason                string `json:"reason,omitempty"`
	Age                   string `json:"age,omitempty"`
	ExitCode              *int   `json:"exitCode,omitempty"`
	LastTerminationReason string `json:"lastTerminationReason,omitempty"`
}

// DeployResponse is returned by POST .../deploy.
type DeployResponse struct {
	BuildID string `json:"buildId"`
}

// BuildInfo is one CI build.
type BuildInfo struct {
	ID         string `json:"id"`
	Phase      string `json:"phase"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	Message    string `json:"message,omitempty"`
}

// --- endpoints (tunnel discovery) ---

// EndpointsResponse lists everything tunnelable in a project.
type EndpointsResponse struct {
	Apps   []AppEndpoints   `json:"apps"`
	Addons []AddonEndpoints `json:"addons"`
}

// AppEndpoints is one app's exposed ports.
type AppEndpoints struct {
	Name  string         `json:"name"`
	Ports []EndpointInfo `json:"ports"`
}

// EndpointInfo is one tunnelable port.
type EndpointInfo struct {
	Port   int      `json:"port"`
	Name   string   `json:"name,omitempty"`
	Routes []string `json:"routes,omitempty"`
}

// AddonEndpoints is one addon's connection port.
type AddonEndpoints struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Port int    `json:"port"`
}

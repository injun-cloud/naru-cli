// Package mcp exposes the naru REST surface as MCP tools so AI agents can drive
// the platform. It reuses the same client + config as the CLI. Read tools carry
// a read-only hint and destructive tools a destructive hint so agents can reason
// about side effects.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/config"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

// Serve starts the stdio MCP server.
func Serve(version string) error {
	// WithInputSchemaValidation enforces each tool's declared required args and
	// types before the handler runs, so a missing/wrong-typed arg returns a clean
	// validation error to the agent instead of a malformed request or a panic.
	s := mcpserver.NewMCPServer("naru", version, mcpserver.WithInputSchemaValidation())
	register(s)
	return mcpserver.ServeStdio(s)
}

func newClient() (*client.Client, error) {
	g, err := config.Resolve()
	if err != nil {
		return nil, err
	}
	if g.Token == "" {
		return nil, fmt.Errorf("not logged in — run `naru login` first")
	}
	return client.New(g.ServerURL, g.Token), nil
}

func errResult(err error) *mcp.CallToolResult { return mcp.NewToolResultError(err.Error()) }

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// getInto does an authenticated GET and returns the decoded body as JSON text.
func getInto[T any](ctx context.Context, path string) (*mcp.CallToolResult, error) {
	cl, err := newClient()
	if err != nil {
		return errResult(err), nil
	}
	var out T
	if err := cl.Get(ctx, path, &out); err != nil {
		return errResult(err), nil
	}
	return jsonResult(out)
}

// write performs a mutating call and returns the response body (or "ok" if empty).
func write(ctx context.Context, method, path string, body any) (*mcp.CallToolResult, error) {
	cl, err := newClient()
	if err != nil {
		return errResult(err), nil
	}
	var raw json.RawMessage
	switch method {
	case "POST":
		err = cl.Post(ctx, path, body, &raw)
	case "PUT":
		err = cl.Put(ctx, path, body, &raw)
	case "PATCH":
		err = cl.Patch(ctx, path, body, &raw)
	case "DELETE":
		err = cl.Delete(ctx, path, &raw)
	default:
		return errResult(fmt.Errorf("bad method %s", method)), nil
	}
	if err != nil {
		return errResult(err), nil
	}
	if len(raw) == 0 {
		return mcp.NewToolResultText("ok"), nil
	}
	return mcp.NewToolResultText(string(raw)), nil
}

// collectLogs reads a bounded (non-following) log stream and joins the lines.
func collectLogs(ctx context.Context, path string) (*mcp.CallToolResult, error) {
	cl, err := newClient()
	if err != nil {
		return errResult(err), nil
	}
	var lines []string
	if err := cl.Stream(ctx, path, func(l string) { lines = append(lines, l) }); err != nil {
		return errResult(err), nil
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func arg(req mcp.CallToolRequest, k string) string { return req.GetString(k, "") }

// logQuery builds a bounded (non-following) log query string consistently with
// the CLI's logQuery, URL-escaping the optional container.
func logQuery(tail, since int, container string, previous bool) string {
	q := url.Values{}
	q.Set("follow", "false")
	q.Set("tail", strconv.Itoa(tail))
	if since > 0 {
		q.Set("since", strconv.Itoa(since))
	}
	if container != "" {
		q.Set("container", container)
	}
	if previous {
		q.Set("previous", "true")
	}
	return "?" + q.Encode()
}

// scalarVars converts the MCP "vars" argument into string secret values. It
// rejects a missing/non-object argument or any non-scalar value (object, array,
// null) so a structured value is never silently formatted into a garbage secret.
func scalarVars(req mcp.CallToolRequest) (map[string]string, error) {
	raw, ok := req.GetArguments()["vars"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'vars' must be an object mapping each KEY to a string value")
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = t
		case bool:
			out[k] = strconv.FormatBool(t)
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case json.Number:
			out[k] = t.String()
		default:
			return nil, fmt.Errorf("value for %q must be a string, number, or boolean", k)
		}
	}
	return out, nil
}

// projApp / projAddon return URL-escaped, path-safe segments so a stray
// '?'/'#'/'/' in a name cannot corrupt the request path.
func projApp(req mcp.CallToolRequest) (string, string) {
	return url.PathEscape(arg(req, "project")), url.PathEscape(arg(req, "app"))
}

func projAddon(req mcp.CallToolRequest) (string, string) {
	return url.PathEscape(arg(req, "project")), url.PathEscape(arg(req, "addon"))
}

// projAPI is the escaped base path for a project.
func projAPI(req mcp.CallToolRequest) string {
	return "/v1/projects/" + url.PathEscape(arg(req, "project"))
}

func ptr[T any](v T) *T { return &v }

// applyToolSchema builds an apply tool's input schema: {project, spec} where the
// spec sub-schema is taken straight from the platform's project schema so the
// agent sees the exact, typed field shape (single source of truth).
func applyToolSchema(specField string) json.RawMessage {
	var root map[string]any
	_ = json.Unmarshal(apitypes.RawSchema(), &root)
	spec := map[string]any{"type": "object"} // fallback
	if props, ok := root["properties"].(map[string]any); ok {
		if f, ok := props[specField].(map[string]any); ok {
			if items, ok := f["items"].(map[string]any); ok {
				spec = items
			}
		}
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": map[string]any{"type": "string", "description": "project name"},
			"spec":    spec,
		},
		"required": []string{"project", "spec"},
	}
	b, _ := json.Marshal(schema)
	return b
}

// specArg re-marshals the structured "spec" object argument into v.
func specArg(req mcp.CallToolRequest, v any) error {
	raw, err := json.Marshal(req.GetArguments()["spec"])
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// upsertApp creates the app if absent, else replaces it (PUT, hash preserved).
func upsertApp(ctx context.Context, cl *client.Client, project string, spec apitypes.AppSpec) (string, error) {
	if spec.Name == "" || spec.Git.Owner == "" || spec.Git.Repo == "" {
		return "", fmt.Errorf("spec needs name and git.owner/git.repo")
	}
	spec.Git.Type = "github"
	if spec.Git.Branch == "" {
		spec.Git.Branch = "main"
	}
	path := fmt.Sprintf("/v1/projects/%s/apps/%s", url.PathEscape(project), url.PathEscape(spec.Name))
	var out apitypes.AppSpec
	if err := cl.Get(ctx, path, &apitypes.AppSpec{}); err == nil {
		req := apitypes.AppUpdateRequest{Git: &spec.Git, Replicas: spec.Replicas, Resources: spec.Resources, Rollout: spec.Rollout, Endpoints: spec.Endpoints}
		return "updated", cl.Put(ctx, path, req, &out)
	} else if !client.NotFound(err) {
		return "", err
	}
	req := apitypes.AppCreateRequest{Name: spec.Name, Git: spec.Git, Replicas: spec.Replicas, Resources: spec.Resources, Rollout: spec.Rollout, Endpoints: spec.Endpoints}
	return "created", cl.Post(ctx, "/v1/projects/"+url.PathEscape(project)+"/apps", req, &out)
}

// upsertAddon creates the addon if absent, else replaces it (type immutable).
func upsertAddon(ctx context.Context, cl *client.Client, project string, spec apitypes.AddonSpec) (string, error) {
	if spec.Name == "" || spec.Type == "" || spec.Version == "" {
		return "", fmt.Errorf("spec needs name, type and version")
	}
	if spec.Size == "" {
		spec.Size = "1Gi"
	}
	req := apitypes.AddonCreateRequest{Name: spec.Name, Type: spec.Type, Version: spec.Version, Size: spec.Size, Resources: spec.Resources}
	if spec.Port > 0 {
		req.Port = &spec.Port
	}
	path := fmt.Sprintf("/v1/projects/%s/addons/%s", url.PathEscape(project), url.PathEscape(spec.Name))
	var out apitypes.AddonSpec
	if err := cl.Get(ctx, path, &apitypes.AddonSpec{}); err == nil {
		return "updated", cl.Put(ctx, path, req, &out)
	} else if !client.NotFound(err) {
		return "", err
	}
	return "created", cl.Post(ctx, "/v1/projects/"+url.PathEscape(project)+"/addons", req, &out)
}

func register(s *mcpserver.MCPServer) {
	// mcp-go always serializes the annotation block with spec defaults
	// (destructiveHint defaults to true), so non-destructive tools must say so
	// explicitly or agents over-warn. ro = read-only, nd = non-destructive write,
	// del = destructive write.
	ro := mcp.WithReadOnlyHintAnnotation(true)
	nd := mcp.WithDestructiveHintAnnotation(false)
	del := mcp.WithDestructiveHintAnnotation(true)

	// --- identity / discovery (read-only) ---

	s.AddTool(mcp.NewTool("whoami",
		mcp.WithDescription("Return the authenticated user's GitHub ID and username."), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[apitypes.MeResponse](ctx, "/v1/auth/me")
		})

	s.AddTool(mcp.NewTool("get_schema",
		mcp.WithDescription("Return the project-YAML JSON schema and its version (field reference for create/update)."), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[apitypes.SchemaResponse](ctx, "/v1/schema")
		})

	// --- projects ---

	s.AddTool(mcp.NewTool("list_projects",
		mcp.WithDescription("List the Naru projects the caller owns (admins see all)."), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[[]apitypes.ProjectSummary](ctx, "/v1/projects")
		})

	s.AddTool(mcp.NewTool("get_project",
		mcp.WithDescription("Get one project (name, applications, addons). Use list_members for owners."),
		mcp.WithString("project", mcp.Required(), mcp.Description("project name")), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[apitypes.Project](ctx, projAPI(req))
		})

	s.AddTool(mcp.NewTool("create_project",
		mcp.WithDescription("Create an empty project. The caller becomes its first owner. "+
			"Name: lowercase letters and digits only (no hyphens); 2-63 chars."),
		mcp.WithString("name", mcp.Required(), mcp.Description("project name")), nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return write(ctx, "POST", "/v1/projects", apitypes.ProjectCreateRequest{Name: arg(req, "name")})
		})

	s.AddTool(mcp.NewTool("delete_project",
		mcp.WithDescription("Delete a project and purge its app secrets from Vault. Irreversible."),
		mcp.WithString("project", mcp.Required()), del),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return write(ctx, "DELETE", projAPI(req), nil)
		})

	// --- members (per-project owners) ---

	s.AddTool(mcp.NewTool("list_members",
		mcp.WithDescription("List a project's owners (GitHub ID + username)."),
		mcp.WithString("project", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[apitypes.MembersResponse](ctx, projAPI(req)+"/members")
		})

	s.AddTool(mcp.NewTool("add_member",
		mcp.WithDescription("Add a GitHub user as a project owner (by username). "+
			"Note: ownership is separate from the platform allowlist — the user must also be allowlisted to log in."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("username", mcp.Required(), mcp.Description("GitHub login to add as owner")), nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return write(ctx, "POST", projAPI(req)+"/members",
				apitypes.AddMemberRequest{Username: arg(req, "username")})
		})

	s.AddTool(mcp.NewTool("remove_member",
		mcp.WithDescription("Remove an owner from a project (by username). The last owner cannot be removed."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("username", mcp.Required()), del),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return write(ctx, "DELETE", projAPI(req)+"/members/"+url.PathEscape(arg(req, "username")), nil)
		})

	// --- applications ---

	s.AddTool(mcp.NewTool("list_apps",
		mcp.WithDescription("List the applications in a project."),
		mcp.WithString("project", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[[]apitypes.AppSpec](ctx, projAPI(req)+"/apps")
		})

	s.AddTool(mcp.NewTool("get_app",
		mcp.WithDescription("Get one application's spec."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return getInto[apitypes.AppSpec](ctx, fmt.Sprintf("/v1/projects/%s/apps/%s", p, a))
		})

	applyApp := mcp.NewToolWithRawSchema("apply_app",
		"Create or update an application (declarative upsert). `spec` is the full app "+
			"spec (name, git, replicas, resources, rollout, endpoints) — its fields match "+
			"`get_schema`. The repo must have the Naru GitHub App installed. The CI-owned "+
			"git hash is preserved; a normal push deploys automatically (no separate deploy needed).",
		applyToolSchema("applications"))
	applyApp.Annotations.DestructiveHint = ptr(false)
	applyApp.Annotations.IdempotentHint = ptr(true)
	s.AddTool(applyApp, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, err := newClient()
		if err != nil {
			return errResult(err), nil
		}
		var spec apitypes.AppSpec
		if err := specArg(req, &spec); err != nil {
			return errResult(err), nil
		}
		action, err := upsertApp(ctx, cl, arg(req, "project"), spec)
		if err != nil {
			return errResult(err), nil
		}
		return mcp.NewToolResultText(action + " app " + spec.Name), nil
	})

	s.AddTool(mcp.NewTool("delete_app",
		mcp.WithDescription("Delete an application and purge its secrets from Vault. Irreversible."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()), del),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return write(ctx, "DELETE", fmt.Sprintf("/v1/projects/%s/apps/%s", p, a), nil)
		})

	// --- status / deploy / logs / builds ---

	s.AddTool(mcp.NewTool("get_app_status",
		mcp.WithDescription("Get an app's live deployment status (phase, replicas, image, pods)."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return getInto[apitypes.StatusInfo](ctx, fmt.Sprintf("/v1/projects/%s/apps/%s/status", p, a))
		})

	s.AddTool(mcp.NewTool("deploy_app",
		mcp.WithDescription("Trigger a build/deploy for an app. Only needed for the first build or a re-deploy "+
			"without a code change — a normal git push deploys automatically."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()), nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return write(ctx, "POST", fmt.Sprintf("/v1/projects/%s/apps/%s/deploy", p, a), nil)
		})

	s.AddTool(mcp.NewTool("promote_app",
		mcp.WithDescription("Promote an app's paused Rollout — approve a manual canary/bluegreen gate, "+
			"fully promoting the new version. Only needed when the app's rollout uses a manual pause."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()), nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return write(ctx, "POST", fmt.Sprintf("/v1/projects/%s/apps/%s/promote", p, a), nil)
		})

	s.AddTool(mcp.NewTool("get_app_logs",
		mcp.WithDescription("Get an app's recent runtime logs (bounded tail, not streaming)."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()),
		mcp.WithNumber("tail", mcp.Description("lines from the end (default 200)")),
		mcp.WithNumber("since", mcp.Description("seconds of history to include")),
		mcp.WithString("container", mcp.Description("container name")),
		mcp.WithBoolean("previous", mcp.Description("logs from the previous (crashed) container instance")), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			q := logQuery(req.GetInt("tail", 200), req.GetInt("since", 0), arg(req, "container"), req.GetBool("previous", false))
			return collectLogs(ctx, fmt.Sprintf("/v1/projects/%s/apps/%s/logs%s", p, a, q))
		})

	s.AddTool(mcp.NewTool("list_builds",
		mcp.WithDescription("List recent CI builds for an app."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return getInto[[]apitypes.BuildInfo](ctx, fmt.Sprintf("/v1/projects/%s/apps/%s/builds", p, a))
		})

	s.AddTool(mcp.NewTool("get_build_logs",
		mcp.WithDescription("Get a build's logs (bounded). Use list_builds to find the build id."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()),
		mcp.WithString("build", mcp.Required(), mcp.Description("build id from list_builds")),
		mcp.WithBoolean("previous", mcp.Description("logs from the previous (crashed) container instance")), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			q := "?follow=false"
			if req.GetBool("previous", false) {
				q += "&previous=true"
			}
			return collectLogs(ctx, fmt.Sprintf("/v1/projects/%s/apps/%s/builds/%s/logs%s", p, a, url.PathEscape(arg(req, "build")), q))
		})

	// --- secrets ---

	s.AddTool(mcp.NewTool("list_secrets",
		mcp.WithDescription("List an app's secret KEYS (values are never returned)."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return getInto[apitypes.SecretKeys](ctx, fmt.Sprintf("/v1/projects/%s/apps/%s/secrets", p, a))
		})

	s.AddTool(mcp.NewTool("set_secret",
		mcp.WithDescription("Set (merge) secrets on an app. They become environment variables; takes effect on the next sync/rollout."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()),
		mcp.WithObject("vars", mcp.Required(), mcp.Description("map of KEY to VALUE")),
		mcp.WithIdempotentHintAnnotation(true), nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			vars, err := scalarVars(req)
			if err != nil {
				return errResult(err), nil
			}
			return write(ctx, "PATCH", fmt.Sprintf("/v1/projects/%s/apps/%s/secrets", p, a), apitypes.SecretVars{Vars: vars})
		})

	s.AddTool(mcp.NewTool("delete_secret",
		mcp.WithDescription("Delete one secret key from an app."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("app", mcp.Required()),
		mcp.WithString("key", mcp.Required()), del),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projApp(req)
			return write(ctx, "DELETE", fmt.Sprintf("/v1/projects/%s/apps/%s/secrets/%s", p, a, url.PathEscape(arg(req, "key"))), nil)
		})

	// --- addons ---

	s.AddTool(mcp.NewTool("list_addons",
		mcp.WithDescription("List a project's addons (databases/caches)."),
		mcp.WithString("project", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[[]apitypes.AddonSpec](ctx, projAPI(req)+"/addons")
		})

	s.AddTool(mcp.NewTool("get_addon",
		mcp.WithDescription("Get one addon's spec (type, version, size, port)."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("addon", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projAddon(req)
			return getInto[apitypes.AddonSpec](ctx, fmt.Sprintf("/v1/projects/%s/addons/%s", p, a))
		})

	s.AddTool(mcp.NewTool("get_addon_status",
		mcp.WithDescription("Get an addon's live deployment status (phase, replicas, image, pods)."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("addon", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projAddon(req)
			return getInto[apitypes.StatusInfo](ctx, fmt.Sprintf("/v1/projects/%s/addons/%s/status", p, a))
		})

	s.AddTool(mcp.NewTool("get_addon_logs",
		mcp.WithDescription("Get an addon's recent logs (bounded tail, not streaming)."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("addon", mcp.Required()),
		mcp.WithNumber("tail", mcp.Description("lines from the end (default 200)")),
		mcp.WithNumber("since", mcp.Description("seconds of history to include")),
		mcp.WithString("container", mcp.Description("container name")),
		mcp.WithBoolean("previous", mcp.Description("logs from the previous (crashed) container instance")), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projAddon(req)
			q := logQuery(req.GetInt("tail", 200), req.GetInt("since", 0), arg(req, "container"), req.GetBool("previous", false))
			return collectLogs(ctx, fmt.Sprintf("/v1/projects/%s/addons/%s/logs%s", p, a, q))
		})

	s.AddTool(mcp.NewTool("get_addon_connection",
		mcp.WithDescription("Get an addon's full connection incl. password (the addon's secret). Fetch this and write the values into an app's secret with set_secret, under whatever key names the app expects."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("addon", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, a := projAddon(req)
			return getInto[apitypes.ConnectionInfo](ctx, fmt.Sprintf("/v1/projects/%s/addons/%s/connection", p, a))
		})

	applyAddon := mcp.NewToolWithRawSchema("apply_addon",
		"Create or update an addon (declarative upsert). `spec` is the full addon spec "+
			"(name, type, version, size, port, resources) — fields match `get_schema`. The "+
			"addon type is immutable. A random password is generated into Vault; reach the addon "+
			"by its name as hostname and wire an app to it with set_secret (env var names are your choice).",
		applyToolSchema("addons"))
	applyAddon.Annotations.DestructiveHint = ptr(false)
	applyAddon.Annotations.IdempotentHint = ptr(true)
	s.AddTool(applyAddon, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, err := newClient()
		if err != nil {
			return errResult(err), nil
		}
		var spec apitypes.AddonSpec
		if err := specArg(req, &spec); err != nil {
			return errResult(err), nil
		}
		action, err := upsertAddon(ctx, cl, arg(req, "project"), spec)
		if err != nil {
			return errResult(err), nil
		}
		return mcp.NewToolResultText(action + " addon " + spec.Name), nil
	})

	s.AddTool(mcp.NewTool("delete_addon",
		mcp.WithDescription("Delete an addon and its data volume. Irreversible."),
		mcp.WithString("project", mcp.Required()),
		mcp.WithString("addon", mcp.Required()), del),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return write(ctx, "DELETE", fmt.Sprintf("/v1/projects/%s/addons/%s", url.PathEscape(arg(req, "project")), url.PathEscape(arg(req, "addon"))), nil)
		})

	// --- endpoints (routing overview) ---

	s.AddTool(mcp.NewTool("list_endpoints",
		mcp.WithDescription("List a project's external routes (host → app:port)."),
		mcp.WithString("project", mcp.Required()), ro, nd),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getInto[apitypes.EndpointsResponse](ctx, projAPI(req)+"/endpoints")
		})
}

// Package mcp exposes the naru REST surface as MCP tools so AI agents can drive
// the platform. It reuses the same client + config as the CLI.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/config"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

// Serve starts the stdio MCP server.
func Serve(version string) error {
	s := mcpserver.NewMCPServer("naru", version)
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

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

func register(s *mcpserver.MCPServer) {
	s.AddTool(
		mcp.NewTool("naru_list_projects", mcp.WithDescription("List all Naru projects")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cl, err := newClient()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var out []apitypes.ProjectSummary
			if err := cl.Get(ctx, "/v1/projects", &out); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(out)
		},
	)

	s.AddTool(
		mcp.NewTool("naru_get_project",
			mcp.WithDescription("Get one Naru project's apps and addons"),
			mcp.WithString("project", mcp.Required(), mcp.Description("project name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cl, err := newClient()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			project := req.GetString("project", "")
			var p apitypes.Project
			if err := cl.Get(ctx, "/v1/projects/"+project, &p); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(p)
		},
	)

	s.AddTool(
		mcp.NewTool("naru_app_status",
			mcp.WithDescription("Get an app's deployment status"),
			mcp.WithString("project", mcp.Required()),
			mcp.WithString("app", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cl, err := newClient()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var st apitypes.StatusDTO
			path := fmt.Sprintf("/v1/projects/%s/apps/%s/status", req.GetString("project", ""), req.GetString("app", ""))
			if err := cl.Get(ctx, path, &st); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(st)
		},
	)

	s.AddTool(
		mcp.NewTool("naru_set_env",
			mcp.WithDescription("Set (merge) environment variables on an app"),
			mcp.WithString("project", mcp.Required()),
			mcp.WithString("app", mcp.Required()),
			mcp.WithObject("vars", mcp.Required(), mcp.Description("map of KEY to VALUE")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cl, err := newClient()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			vars := map[string]string{}
			for k, v := range req.GetArguments()["vars"].(map[string]any) {
				vars[k] = fmt.Sprint(v)
			}
			path := fmt.Sprintf("/v1/projects/%s/apps/%s/env", req.GetString("project", ""), req.GetString("app", ""))
			if err := cl.Patch(ctx, path, apitypes.EnvVars{Vars: vars}, nil); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("ok"), nil
		},
	)

	s.AddTool(
		mcp.NewTool("naru_deploy",
			mcp.WithDescription("Trigger a build/deploy for an app"),
			mcp.WithString("project", mcp.Required()),
			mcp.WithString("app", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cl, err := newClient()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var out apitypes.DeployResponse
			path := fmt.Sprintf("/v1/projects/%s/apps/%s/deploy", req.GetString("project", ""), req.GetString("app", ""))
			if err := cl.Post(ctx, path, nil, &out); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(out)
		},
	)

	s.AddTool(
		mcp.NewTool("naru_list_builds",
			mcp.WithDescription("List recent builds for an app"),
			mcp.WithString("project", mcp.Required()),
			mcp.WithString("app", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cl, err := newClient()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var out []apitypes.BuildInfo
			path := fmt.Sprintf("/v1/projects/%s/apps/%s/builds", req.GetString("project", ""), req.GetString("app", ""))
			if err := cl.Get(ctx, path, &out); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(out)
		},
	)

	s.AddTool(
		mcp.NewTool("naru_addon_conn",
			mcp.WithDescription("Get an addon's connection info (no password)"),
			mcp.WithString("project", mcp.Required()),
			mcp.WithString("addon", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cl, err := newClient()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var dto apitypes.ConnectionDTO
			path := fmt.Sprintf("/v1/projects/%s/addons/%s/connection", req.GetString("project", ""), req.GetString("addon", ""))
			if err := cl.Get(ctx, path, &dto); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(dto)
		},
	)
}

package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/injun-cloud/naru-cli/internal/apitypes"
)

// UpsertApp creates the app if absent, else replaces it (PUT; the server preserves
// the CI-owned git hash). Returns "created"/"updated" and the server-returned spec.
// Shared by the CLI and the MCP server so the probe + defaults + paths live once.
func (c *Client) UpsertApp(ctx context.Context, project string, spec apitypes.AppSpec) (string, apitypes.AppSpec, error) {
	var out apitypes.AppSpec
	if spec.Name == "" || spec.Git.Owner == "" || spec.Git.Repo == "" {
		return "", out, fmt.Errorf("spec needs name and git.owner/git.repo")
	}
	spec.Git.Type = "github"
	if spec.Git.Branch == "" {
		spec.Git.Branch = "main"
	}
	base := "/v1/projects/" + url.PathEscape(project)
	item := base + "/apps/" + url.PathEscape(spec.Name)
	if err := c.Get(ctx, item, &apitypes.AppSpec{}); err == nil {
		req := apitypes.AppUpdateRequest{Git: &spec.Git, Replicas: spec.Replicas, Resources: spec.Resources, Rollout: spec.Rollout, Endpoints: spec.Endpoints}
		if err := c.Put(ctx, item, req, &out); err != nil {
			return "", out, err
		}
		return "updated", out, nil
	} else if !NotFound(err) {
		return "", out, err
	}
	req := apitypes.AppCreateRequest{Name: spec.Name, Git: spec.Git, Replicas: spec.Replicas, Resources: spec.Resources, Rollout: spec.Rollout, Endpoints: spec.Endpoints}
	if err := c.Post(ctx, base+"/apps", req, &out); err != nil {
		return "", out, err
	}
	return "created", out, nil
}

// UpsertAddon creates the addon if absent, else replaces it (PUT; type is
// immutable). Size defaults to 1Gi. Returns "created"/"updated" and the spec.
func (c *Client) UpsertAddon(ctx context.Context, project string, spec apitypes.AddonSpec) (string, apitypes.AddonSpec, error) {
	var out apitypes.AddonSpec
	if spec.Name == "" || spec.Type == "" || spec.Version == "" {
		return "", out, fmt.Errorf("spec needs name, type and version")
	}
	if spec.Size == "" {
		spec.Size = "1Gi"
	}
	req := apitypes.AddonCreateRequest{Name: spec.Name, Type: spec.Type, Version: spec.Version, Size: spec.Size, Resources: spec.Resources}
	if spec.Port > 0 {
		req.Port = &spec.Port
	}
	base := "/v1/projects/" + url.PathEscape(project)
	item := base + "/addons/" + url.PathEscape(spec.Name)
	if err := c.Get(ctx, item, &apitypes.AddonSpec{}); err == nil {
		if err := c.Put(ctx, item, req, &out); err != nil {
			return "", out, err
		}
		return "updated", out, nil
	} else if !NotFound(err) {
		return "", out, err
	}
	if err := c.Post(ctx, base+"/addons", req, &out); err != nil {
		return "", out, err
	}
	return "created", out, nil
}

// LogQuery builds the query string for a log stream, shared by the CLI and MCP so
// the parameter encoding can't drift. since/container are omitted when unset.
func LogQuery(follow bool, tail, since int, container string, previous bool) string {
	q := url.Values{}
	q.Set("follow", strconv.FormatBool(follow))
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

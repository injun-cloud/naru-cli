package cmd

import (
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func appPath(project, app string) string {
	return fmt.Sprintf("/v1/projects/%s/apps/%s", url.PathEscape(project), url.PathEscape(app))
}

func newAppCmd() *cobra.Command {
	c := &cobra.Command{Use: "app", Aliases: []string{"a"}, Short: "Manage applications"}
	c.AddCommand(
		appListCmd(), appCreateCmd(), appGetCmd(), appEditCmd(), appApplyCmd(), appRmCmd(),
		appStatusCmd(), appLogsCmd(), appDeployCmd(), appPromoteCmd(), appBuildsCmd(), appTunnelCmd(),
	)
	return c
}

// upsertApp creates the app if absent, otherwise replaces it (PUT). The CI-owned
// git hash is preserved server-side. It returns "created" or "updated".
func upsertApp(cmd *cobra.Command, cl *client.Client, project string, spec apitypes.AppSpec) (string, error) {
	if spec.Name == "" {
		return "", fmt.Errorf("spec is missing 'name'")
	}
	if spec.Git.Owner == "" || spec.Git.Repo == "" {
		return "", fmt.Errorf("spec is missing git.owner/git.repo")
	}
	spec.Git.Type = "github"
	if spec.Git.Branch == "" {
		spec.Git.Branch = "main"
	}
	var out apitypes.AppSpec
	err := cl.Get(cmd.Context(), appPath(project, spec.Name), &apitypes.AppSpec{})
	if err == nil {
		req := apitypes.AppUpdateRequest{Git: &spec.Git, Replicas: spec.Replicas, Resources: spec.Resources, Rollout: spec.Rollout, Endpoints: spec.Endpoints}
		if err := cl.Put(cmd.Context(), appPath(project, spec.Name), req, &out); err != nil {
			return "", err
		}
		return "updated", nil
	}
	if !client.NotFound(err) {
		return "", err
	}
	req := apitypes.AppCreateRequest{Name: spec.Name, Git: spec.Git, Replicas: spec.Replicas, Resources: spec.Resources, Rollout: spec.Rollout, Endpoints: spec.Endpoints}
	if err := cl.Post(cmd.Context(), "/v1/projects/"+url.PathEscape(project)+"/apps", req, &out); err != nil {
		return "", err
	}
	return "created", nil
}

func appListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "ls", Aliases: []string{"list"}, Short: "List apps",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var apps []apitypes.AppSpec
			if err := cl.Get(cmd.Context(), "/v1/projects/"+url.PathEscape(project)+"/apps", &apps); err != nil {
				return err
			}
			return printer().Emit(apps, func() {
				rows := make([][]string, 0, len(apps))
				for _, a := range apps {
					hash := a.Git.Hash
					if hash == "" {
						hash = "(not built)"
					} else if len(hash) > 7 {
						hash = hash[:7]
					}
					rows = append(rows, []string{a.Name, a.Git.Owner + "/" + a.Git.Repo, a.Git.Branch, hash})
				}
				output.Table([]string{"NAME", "REPO", "BRANCH", "HASH"}, rows)
			})
		},
	}
}

func appCreateCmd() *cobra.Command {
	var repo, branch string
	c := &cobra.Command{
		Use: "create <name>", Short: "Create an app (minimal bootstrap)", Args: cobra.ExactArgs(1),
		Long: "Bootstrap an app from a repo. For the full spec (replicas, resources,\n" +
			"rollout, endpoints) use `naru app edit` or `naru app apply -f`.",
		Example: "  naru app create api --repo in-jun/api -p myproj\n" +
			"  naru app create web --repo in-jun/web --branch dev -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			owner, name, ok := strings.Cut(repo, "/")
			if !ok {
				return fmt.Errorf("--repo must be owner/repo")
			}
			spec := apitypes.AppSpec{
				Name: args[0],
				Git:  apitypes.GitSpec{Type: "github", Owner: owner, Repo: name, Branch: branch},
			}
			action, err := upsertApp(cmd, cl, project, spec)
			if err != nil {
				return err
			}
			output.Success(action + " app " + args[0] + " — edit it or push to its repo to build")
			return nil
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "owner/repo (required)")
	c.Flags().StringVar(&branch, "branch", "", "git branch (default: main)")
	_ = c.MarkFlagRequired("repo")
	return c
}

func appGetCmd() *cobra.Command {
	var outFmt string
	c := &cobra.Command{
		Use: "get <name>", Short: "Show an app (-o yaml for the editable spec)", Args: cobra.ExactArgs(1),
		Example: "  naru app get api -p myproj\n" +
			"  naru app get api -o yaml > api.yaml   # edit, then: naru app apply -f api.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var a apitypes.AppSpec
			if err := cl.Get(cmd.Context(), appPath(project, args[0]), &a); err != nil {
				return err
			}
			if outFmt == "yaml" {
				b, err := marshalSpecYAML(a)
				if err != nil {
					return err
				}
				fmt.Print(string(b))
				return nil
			}
			return printer().Emit(a, func() {
				fmt.Printf("name:   %s\nrepo:   %s/%s\nbranch: %s\nhash:   %s\n",
					a.Name, a.Git.Owner, a.Git.Repo, a.Git.Branch, a.Git.Hash)
				for _, ep := range a.Endpoints {
					fmt.Printf("port:   %d %s\n", ep.Port, strings.Join(ep.Routes, ", "))
				}
			})
		},
	}
	c.Flags().StringVarP(&outFmt, "output", "o", "", "output format: yaml")
	return c
}

func appEditCmd() *cobra.Command {
	return &cobra.Command{
		Use: "edit <name>", Short: "Edit an app's full spec in $EDITOR", Args: cobra.ExactArgs(1),
		Example: "  naru app edit api -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagNoInput {
				return fmt.Errorf("edit needs an interactive editor; use `naru app apply -f` instead")
			}
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var cur apitypes.AppSpec
			if err := cl.Get(cmd.Context(), appPath(project, args[0]), &cur); err != nil {
				return err
			}
			cur.Git.Hash = "" // CI-owned; not user-editable
			initial, err := marshalSpecYAML(cur)
			if err != nil {
				return err
			}
			edited, err := editInEditor(initial, "yaml")
			if err != nil {
				return err
			}
			if edited == nil {
				output.Info("no changes")
				return nil
			}
			var spec apitypes.AppSpec
			if err := yamlUnmarshal(edited, &spec); err != nil {
				return err
			}
			spec.Name = args[0] // name is immutable
			action, err := upsertApp(cmd, cl, project, spec)
			if err != nil {
				return err
			}
			output.Success(action + " app " + args[0])
			return nil
		},
	}
}

func appApplyCmd() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use: "apply", Short: "Create or update an app from a spec file (-f)",
		Example: "  naru app apply -f api.yaml -p myproj\n" +
			"  naru app get api -o yaml | yq '.replicas = 3' | naru app apply -f - -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var spec apitypes.AppSpec
			if err := loadSpecFile(file, &spec); err != nil {
				return err
			}
			action, err := upsertApp(cmd, cl, project, spec)
			if err != nil {
				return err
			}
			output.Success(action + " app " + spec.Name)
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "spec file (YAML/JSON, - for stdin)")
	_ = c.MarkFlagRequired("file")
	return c
}

func appRmCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rm <name>", Aliases: []string{"delete"}, Short: "Delete an app", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			if err := cl.Delete(cmd.Context(), appPath(project, args[0]), nil); err != nil {
				return err
			}
			output.Success("deleted app " + args[0])
			return nil
		},
	}
}

func appStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use: "status <name>", Short: "Show deployment status", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var st apitypes.StatusInfo
			if err := cl.Get(cmd.Context(), appPath(project, args[0])+"/status", &st); err != nil {
				return err
			}
			return printer().Emit(st, func() {
				fmt.Printf("phase: %s  ready: %d/%d  image: %s\n", st.Phase, st.Ready, st.Desired, st.Image)
				rows := make([][]string, 0, len(st.Pods))
				for _, p := range st.Pods {
					rows = append(rows, []string{p.Name, p.Phase, strconv.FormatBool(p.Ready), strconv.Itoa(p.Restarts), p.Reason})
				}
				output.Table([]string{"POD", "PHASE", "READY", "RESTARTS", "REASON"}, rows)
			})
		},
	}
}

// logQuery builds the query string for a log stream, URL-escaping the optional
// container so a value containing & or # cannot corrupt or inject query params.
func logQuery(follow bool, tail, since int, container string, previous bool) string {
	q := url.Values{}
	q.Set("follow", strconv.FormatBool(follow))
	q.Set("tail", strconv.Itoa(tail))
	q.Set("since", strconv.Itoa(since))
	if container != "" {
		q.Set("container", container)
	}
	if previous {
		q.Set("previous", "true")
	}
	return "?" + q.Encode()
}

func appLogsCmd() *cobra.Command {
	var follow, previous bool
	var tail, since int
	var container string
	c := &cobra.Command{
		Use: "logs <name>", Short: "Stream app logs", Args: cobra.ExactArgs(1),
		Example: "  naru app logs api -p myproj --tail 200\n  naru app logs api -f   # follow",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			return cl.Stream(cmd.Context(), appPath(project, args[0])+"/logs"+logQuery(follow, tail, since, container, previous), func(line string) {
				fmt.Println(line)
			})
		},
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "follow")
	c.Flags().IntVar(&tail, "tail", 100, "lines from the end")
	c.Flags().IntVar(&since, "since", 0, "seconds of history")
	c.Flags().StringVar(&container, "container", "", "container name")
	c.Flags().BoolVar(&previous, "previous", false, "logs from the previous (crashed) container instance")
	return c
}

func appDeployCmd() *cobra.Command {
	return &cobra.Command{
		Use: "deploy <name>", Short: "Trigger a build/deploy", Args: cobra.ExactArgs(1),
		Long: "Trigger a build/deploy. Only needed for the first build or a re-deploy\n" +
			"without a code change — a normal `git push` deploys automatically.",
		Example: "  naru app deploy api -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var out apitypes.DeployResponse
			if err := cl.Post(cmd.Context(), appPath(project, args[0])+"/deploy", nil, &out); err != nil {
				return err
			}
			output.Success("build started: " + out.BuildID)
			return nil
		},
	}
}

func appPromoteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "promote <name>", Short: "Promote a paused rollout (approve a manual canary/bluegreen gate)", Args: cobra.ExactArgs(1),
		Long: "Resume a Rollout paused at a manual gate (canary pause / bluegreen manual\n" +
			"promote), fully promoting the new version.",
		Example: "  naru app promote api -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			if err := cl.Post(cmd.Context(), appPath(project, args[0])+"/promote", nil, nil); err != nil {
				return err
			}
			output.Success("promoted " + args[0])
			return nil
		},
	}
}

func appBuildsCmd() *cobra.Command {
	var follow bool
	c := &cobra.Command{
		Use: "builds <name> [buildId]", Short: "List builds, or stream one's logs", Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			if len(args) == 2 {
				if follow {
					return cl.Stream(cmd.Context(), appPath(project, args[0])+"/builds/"+url.PathEscape(args[1])+"/logs?follow=true", func(l string) { fmt.Println(l) })
				}
				var b apitypes.BuildInfo
				if err := cl.Get(cmd.Context(), appPath(project, args[0])+"/builds/"+url.PathEscape(args[1]), &b); err != nil {
					return err
				}
				return printer().Emit(b, func() { fmt.Printf("%s  %s  %s\n", b.ID, b.Phase, b.Message) })
			}
			var builds []apitypes.BuildInfo
			if err := cl.Get(cmd.Context(), appPath(project, args[0])+"/builds", &builds); err != nil {
				return err
			}
			return printer().Emit(builds, func() {
				rows := make([][]string, 0, len(builds))
				for _, b := range builds {
					rows = append(rows, []string{b.ID, b.Phase, b.StartedAt})
				}
				output.Table([]string{"BUILD", "PHASE", "STARTED"}, rows)
			})
		},
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "stream build logs")
	return c
}

func appTunnelCmd() *cobra.Command {
	var port, localPort int
	c := &cobra.Command{
		Use: "tunnel <name>", Short: "Tunnel a local port to an app endpoint", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			if port == 0 {
				return fmt.Errorf("--port is required")
			}
			if localPort == 0 {
				localPort = port
			}
			path := fmt.Sprintf("%s/tunnel?port=%d", appPath(project, args[0]), port)
			return runTunnel(cmd, cl, path, localPort)
		},
	}
	c.Flags().IntVar(&port, "port", 0, "target app endpoint port (required)")
	c.Flags().IntVar(&localPort, "local-port", 0, "local port (default: same as --port)")
	return c
}

// runTunnel opens the local listener and blocks until interrupted.
func runTunnel(cmd *cobra.Command, cl *client.Client, path string, localPort int) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	return cl.RunTunnel(ctx, path, addr, func(actual string) {
		output.Info("tunnel listening on " + actual + " — Ctrl-C to stop")
	})
}

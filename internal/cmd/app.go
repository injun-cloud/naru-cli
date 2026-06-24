package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

// resourceFromFlags builds a ResourceSpec from cpu/memory request+limit flags,
// returning nil if none are set so it doesn't clobber an existing spec.
func resourceFromFlags(cpuReq, memReq, cpuLim, memLim string) *apitypes.ResourceSpec {
	rs := &apitypes.ResourceSpec{}
	put := func(m *map[string]string, k, v string) {
		if v == "" {
			return
		}
		if *m == nil {
			*m = map[string]string{}
		}
		(*m)[k] = v
	}
	put(&rs.Requests, "cpu", cpuReq)
	put(&rs.Requests, "memory", memReq)
	put(&rs.Limits, "cpu", cpuLim)
	put(&rs.Limits, "memory", memLim)
	if rs.Requests == nil && rs.Limits == nil {
		return nil
	}
	return rs
}

// addResourceFlags registers the shared cpu/memory request+limit flags.
func addResourceFlags(c *cobra.Command, cpuReq, memReq, cpuLim, memLim *string) {
	c.Flags().StringVar(cpuReq, "cpu", "", "CPU request, e.g. 100m")
	c.Flags().StringVar(memReq, "memory", "", "memory request, e.g. 128Mi")
	c.Flags().StringVar(cpuLim, "cpu-limit", "", "CPU limit")
	c.Flags().StringVar(memLim, "memory-limit", "", "memory limit")
}

// loadSpecFile unmarshals a YAML or JSON spec file into v (YAML is a JSON superset).
func loadSpecFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func appPath(project, app string) string {
	return fmt.Sprintf("/v1/projects/%s/apps/%s", project, app)
}

func newAppCmd() *cobra.Command {
	c := &cobra.Command{Use: "app", Aliases: []string{"a"}, Short: "Manage applications"}
	c.AddCommand(
		appListCmd(), appCreateCmd(), appGetCmd(), appSetCmd(), appRmCmd(),
		appStatusCmd(), appLogsCmd(), appDeployCmd(), appBuildsCmd(), appTunnelCmd(),
	)
	return c
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
			if err := cl.Get(cmd.Context(), "/v1/projects/"+project+"/apps", &apps); err != nil {
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
	var repo, branch, rollout, file string
	var cpuReq, memReq, cpuLim, memLim string
	var ports []string
	var replicas int
	c := &cobra.Command{
		Use: "create <name>", Short: "Create an app", Args: cobra.ExactArgs(1),
		Long: "Create an app. Use flags for the common fields, or -f to supply a full\n" +
			"spec (resources, rollout steps, etc. — see `naru schema`); flags override the file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var req apitypes.AppCreateRequest
			if file != "" {
				if err := loadSpecFile(file, &req); err != nil {
					return err
				}
			}
			req.Name = args[0]
			req.Git.Type = "github"
			if repo != "" {
				owner, name, ok := strings.Cut(repo, "/")
				if !ok {
					return fmt.Errorf("--repo must be owner/repo")
				}
				req.Git.Owner, req.Git.Repo = owner, name
			}
			if req.Git.Owner == "" || req.Git.Repo == "" {
				return fmt.Errorf("repo is required (set --repo owner/repo or git.owner/git.repo in -f)")
			}
			if branch != "" {
				req.Git.Branch = branch
			}
			if req.Git.Branch == "" {
				req.Git.Branch = "main"
			}
			if len(ports) > 0 {
				if req.Endpoints, err = parsePorts(ports); err != nil {
					return err
				}
			}
			if replicas > 0 {
				req.Replicas = &replicas
			}
			if rollout != "" {
				if req.Rollout == nil {
					req.Rollout = &apitypes.RolloutSpec{}
				}
				req.Rollout.Strategy = rollout
			}
			if rs := resourceFromFlags(cpuReq, memReq, cpuLim, memLim); rs != nil {
				req.Resources = rs
			}
			var out apitypes.AppSpec
			if err := cl.Post(cmd.Context(), "/v1/projects/"+project+"/apps", req, &out); err != nil {
				return err
			}
			output.Success("created app " + args[0] + " — push to its repo to trigger the first build")
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "spec file (YAML/JSON) for full-schema fields")
	c.Flags().StringVar(&repo, "repo", "", "owner/repo")
	c.Flags().StringVar(&branch, "branch", "", "git branch (default: main)")
	c.Flags().StringArrayVar(&ports, "port", nil, "PORT[:host[/path]] (repeatable)")
	c.Flags().IntVar(&replicas, "replicas", 0, "replica count")
	c.Flags().StringVar(&rollout, "rollout", "", "rollout strategy: rolling|canary|bluegreen")
	addResourceFlags(c, &cpuReq, &memReq, &cpuLim, &memLim)
	return c
}

func appGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get <name>", Short: "Show an app", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var a apitypes.AppSpec
			if err := cl.Get(cmd.Context(), appPath(project, args[0]), &a); err != nil {
				return err
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
}

func appSetCmd() *cobra.Command {
	var branch, rollout, file string
	var cpuReq, memReq, cpuLim, memLim string
	var ports []string
	var replicas int
	c := &cobra.Command{
		Use: "set <name>", Short: "Update an app (PATCH)", Args: cobra.ExactArgs(1),
		Long: "Update an app. Only the fields you pass change. Use -f for full-schema\n" +
			"fields (resources, rollout steps, …); flags override the file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var req apitypes.AppUpdateRequest
			if file != "" {
				if err := loadSpecFile(file, &req); err != nil {
					return err
				}
			}
			if branch != "" {
				// fetch current git to preserve owner/repo
				var cur apitypes.AppSpec
				if err := cl.Get(cmd.Context(), appPath(project, args[0]), &cur); err != nil {
					return err
				}
				cur.Git.Branch = branch
				req.Git = &cur.Git
			}
			if replicas > 0 {
				req.Replicas = &replicas
			}
			if rollout != "" {
				if req.Rollout == nil {
					req.Rollout = &apitypes.RolloutSpec{}
				}
				req.Rollout.Strategy = rollout
			}
			if len(ports) > 0 {
				if req.Endpoints, err = parsePorts(ports); err != nil {
					return err
				}
			}
			if rs := resourceFromFlags(cpuReq, memReq, cpuLim, memLim); rs != nil {
				req.Resources = rs
			}
			var out apitypes.AppSpec
			if err := cl.Patch(cmd.Context(), appPath(project, args[0]), req, &out); err != nil {
				return err
			}
			output.Success("updated app " + args[0])
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "spec file (YAML/JSON) for full-schema fields")
	c.Flags().StringVar(&branch, "branch", "", "git branch")
	c.Flags().StringArrayVar(&ports, "port", nil, "PORT[:host[/path]] (replaces endpoints)")
	c.Flags().IntVar(&replicas, "replicas", 0, "replica count")
	c.Flags().StringVar(&rollout, "rollout", "", "rollout strategy")
	addResourceFlags(c, &cpuReq, &memReq, &cpuLim, &memLim)
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
			var st apitypes.StatusDTO
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

func appLogsCmd() *cobra.Command {
	var follow bool
	var tail, since int
	var container string
	c := &cobra.Command{
		Use: "logs <name>", Short: "Stream app logs", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			q := fmt.Sprintf("?follow=%t&tail=%d&since=%d&container=%s", follow, tail, since, container)
			return cl.Stream(cmd.Context(), appPath(project, args[0])+"/logs"+q, func(line string) {
				fmt.Println(line)
			})
		},
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "follow")
	c.Flags().IntVar(&tail, "tail", 100, "lines from the end")
	c.Flags().IntVar(&since, "since", 0, "seconds of history")
	c.Flags().StringVar(&container, "container", "", "container name")
	return c
}

func appDeployCmd() *cobra.Command {
	return &cobra.Command{
		Use: "deploy <name>", Short: "Trigger a build/deploy", Args: cobra.ExactArgs(1),
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
					return cl.Stream(cmd.Context(), appPath(project, args[0])+"/builds/"+args[1]+"/logs?follow=true", func(l string) { fmt.Println(l) })
				}
				var b apitypes.BuildInfo
				if err := cl.Get(cmd.Context(), appPath(project, args[0])+"/builds/"+args[1], &b); err != nil {
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

// parsePorts converts "PORT[:host[/path]]" strings into endpoints.
func parsePorts(ports []string) ([]apitypes.EndpointSpec, error) {
	var eps []apitypes.EndpointSpec
	for _, p := range ports {
		portStr, route, hasRoute := strings.Cut(p, ":")
		n, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q", p)
		}
		ep := apitypes.EndpointSpec{Port: n}
		if hasRoute && route != "" {
			ep.Routes = []string{route}
		}
		eps = append(eps, ep)
	}
	return eps, nil
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

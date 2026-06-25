package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/config"
	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := config.Resolve()
			if err != nil {
				return err
			}
			if flagServer != "" {
				g.ServerURL = flagServer
			}
			c := client.New(g.ServerURL, "")

			var cfg apitypes.AuthConfig
			if err := c.Get(cmd.Context(), "/v1/auth/config", &cfg); err != nil {
				return fmt.Errorf("fetch auth config: %w", err)
			}

			code, err := receiveOAuthCode(cmd.Context(), cfg.ClientID)
			if err != nil {
				return err
			}
			var resp apitypes.AuthResponse
			if err := c.Post(cmd.Context(), "/v1/auth/exchange", apitypes.ExchangeRequest{Code: code}, &resp); err != nil {
				return err
			}
			g.Token = resp.Token
			g.Username = resp.Username
			if err := config.SaveGlobal(g); err != nil {
				return err
			}
			output.Success("logged in as " + resp.Username)
			return nil
		},
	}
}

// receiveOAuthCode runs a loopback callback server and returns the OAuth code.
// The CLI owns the CSRF state and the redirect, keeping the server stateless.
func receiveOAuthCode(ctx context.Context, clientID string) (string, error) {
	// Don't wait forever if the user never completes (or never opens) the flow.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	port := ln.Addr().(*net.TCPAddr).Port

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		ln.Close()
		return "", err
	}
	state := hex.EncodeToString(stateBytes)
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	authURL := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&state=%s",
		clientID, redirect, state)

	output.Info("Open this URL in your browser to authorize:")
	output.Info("  " + authURL)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{}
	mux := http.NewServeMux()
	srv.Handler = mux
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth state mismatch (possible CSRF)")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			if e := r.URL.Query().Get("error"); e != "" {
				http.Error(w, "authorization failed: "+e, http.StatusBadRequest)
				errCh <- fmt.Errorf("authorization failed: %s", e)
				return
			}
			http.Error(w, "no code", http.StatusBadRequest)
			errCh <- fmt.Errorf("no code in callback")
			return
		}
		fmt.Fprintln(w, "Authentication successful — you can close this tab.")
		codeCh <- code
	})
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timed out waiting for authorization (5m)")
		}
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case code := <-codeCh:
		return code, nil
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			g.Token = ""
			g.Username = ""
			if err := config.SaveGlobal(g); err != nil {
				return err
			}
			output.Success("logged out")
			return nil
		},
	}
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated user",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			var me apitypes.MeResponse
			if err := c.Get(cmd.Context(), "/v1/auth/me", &me); err != nil {
				return err
			}
			return printer().Emit(me, func() {
				fmt.Printf("%s (id %d)\n", me.Username, me.GithubID)
			})
		},
	}
}

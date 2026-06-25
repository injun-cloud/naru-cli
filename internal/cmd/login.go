package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
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
			writeAuthPage(w, http.StatusBadRequest, false, "Authentication failed",
				"State mismatch (possible CSRF). Run <code>naru login</code> again.")
			errCh <- fmt.Errorf("oauth state mismatch (possible CSRF)")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			if e := r.URL.Query().Get("error"); e != "" {
				writeAuthPage(w, http.StatusBadRequest, false, "Authorization denied",
					html.EscapeString(e)+" — run <code>naru login</code> again.")
				errCh <- fmt.Errorf("authorization failed: %s", e)
				return
			}
			writeAuthPage(w, http.StatusBadRequest, false, "Authentication failed",
				"No authorization code was returned. Run <code>naru login</code> again.")
			errCh <- fmt.Errorf("no code in callback")
			return
		}
		writeAuthPage(w, http.StatusOK, true, "You're signed in",
			"Authentication succeeded. You can close this tab and return to the terminal.")
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

// writeAuthPage renders the small self-contained HTML page the browser lands on
// after the OAuth redirect. detail may contain trusted HTML (callers escape any
// user-controlled value).
func writeAuthPage(w http.ResponseWriter, status int, ok bool, heading, detail string) {
	accent, icon := "#16a34a", "&#10003;" // green check
	if !ok {
		accent, icon = "#dc2626", "&#10005;" // red cross
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Naru &middot; %[3]s</title></head>
<body style="margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#0b0f17;color:#e5e7eb;font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif">
<div style="text-align:center;padding:44px 52px;background:#111827;border:1px solid #1f2937;border-radius:18px;box-shadow:0 16px 50px rgba(0,0,0,.45);max-width:400px">
<div style="width:66px;height:66px;margin:0 auto 22px;border-radius:50%%;background:%[1]s;display:flex;align-items:center;justify-content:center;font-size:32px;color:#fff">%[2]s</div>
<div style="font-size:12px;letter-spacing:.22em;text-transform:uppercase;color:#6b7280;margin-bottom:8px">naru</div>
<h1 style="font-size:21px;margin:0 0 10px;font-weight:600">%[3]s</h1>
<p style="margin:0;color:#9ca3af;font-size:14px;line-height:1.6">%[4]s</p>
</div></body></html>`, accent, icon, html.EscapeString(heading), detail)
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

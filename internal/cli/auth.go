package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type cliLoginStartResponse struct {
	LoginID       string    `json:"loginId"`
	StartURL      string    `json:"startUrl"`
	ExchangeToken string    `json:"exchangeToken"`
	IntervalMS    int       `json:"intervalMs"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type cliLoginExchangeResponse struct {
	Status   string         `json:"status"`
	User     SessionUser    `json:"user"`
	Token    string         `json:"token"`
	Metadata APITokenRecord `json:"metadata"`
}

type SessionUser struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	Email       string `json:"email,omitempty"`
}

type authSessionResponse struct {
	User SessionUser `json:"user"`
}

type APITokenRecord struct {
	ID          string `json:"id"`
	TokenPrefix string `json:"tokenPrefix"`
	CreatedAt   string `json:"createdAt"`
	RevokedAt   string `json:"revokedAt,omitempty"`
}

type cliLoginExchangeRequest struct {
	ExchangeToken string `json:"exchangeToken"`
}

func newAuthCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate the CLI against Splat",
	}
	cmd.AddCommand(newAuthLoginCmd(opts))
	cmd.AddCommand(newAuthStatusCmd(opts))
	cmd.AddCommand(newAuthLogoutCmd(opts))
	return cmd
}

func newAuthLoginCmd(opts *rootOptions) *cobra.Command {
	var withToken bool
	var web bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Splat and store an API token locally",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if withToken {
				return loginWithToken(cmd, opts)
			}
			if !web {
				return fmt.Errorf("only the web login flow is supported unless you pass --with-token")
			}
			return loginWithBrowser(cmd, opts)
		},
	}

	cmd.Flags().BoolVar(&withToken, "with-token", false, "Read a Splat API token from standard input")
	cmd.Flags().BoolVar(&web, "web", true, "Open the Splat UI in a browser to authorize this CLI")
	return cmd
}

func newAuthStatusCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current authenticated Splat user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := opts.client()
			var response authSessionResponse
			if err := client.Get(cmd.Context(), "/auth/session", &response); err != nil {
				return err
			}
			return printJSON(cmd, response)
		},
	}
}

func newAuthLogoutCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the locally stored Splat API token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadCLIConfig(opts.configPath)
			if err != nil {
				return err
			}
			cfg.Token = ""
			if err := saveCLIConfig(opts.configPath, cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Logged out of Splat CLI.")
			return nil
		},
	}
}

func loginWithToken(cmd *cobra.Command, opts *rootOptions) error {
	reader := bufio.NewReader(cmd.InOrStdin())
	token, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token is required")
	}

	client := opts.clientWithToken(token)
	var response authSessionResponse
	if err := client.Get(cmd.Context(), "/auth/session", &response); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	if err := opts.saveConfig(token); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Logged in to Splat as %s.\n", response.User.DisplayName)
	return nil
}

func loginWithBrowser(cmd *cobra.Command, opts *rootOptions) error {
	client := opts.clientWithoutToken()
	var start cliLoginStartResponse
	if err := client.Post(cmd.Context(), "/auth/cli/login", map[string]any{}, &start); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Open this URL to authorize the CLI:\n%s\n", start.StartURL)
	if err := openBrowser(start.StartURL); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Failed to open browser automatically: %v\n", err)
	}

	interval := time.Duration(start.IntervalMS) * time.Millisecond
	if interval <= 0 {
		interval = time.Second
	}

	deadline := start.ExpiresAt
	for {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return fmt.Errorf("cli login expired before completion")
		}

		var response cliLoginExchangeResponse
		err := client.Post(cmd.Context(), "/auth/cli/login/"+url.PathEscape(start.LoginID)+"/exchange", cliLoginExchangeRequest{
			ExchangeToken: start.ExchangeToken,
		}, &response)
		if err == nil && response.Status == "complete" {
			if err := opts.saveConfig(response.Token); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Logged in to Splat as %s.\n", response.User.DisplayName)
			return nil
		}
		if err == nil {
			time.Sleep(interval)
			continue
		}
		if strings.Contains(err.Error(), "request failed with 202") || strings.Contains(err.Error(), "pending") {
			time.Sleep(interval)
			continue
		}
		return err
	}
}

func openBrowser(target string) error {
	var command string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{target}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", target}
	default:
		command = "xdg-open"
		args = []string{target}
	}

	return exec.Command(command, args...).Start()
}

func (o *rootOptions) saveConfig(token string) error {
	cfg, err := loadCLIConfig(o.configPath)
	if err != nil {
		return err
	}
	cfg.Token = strings.TrimSpace(token)
	if apiURL := o.resolvedAPIURL(); apiURL != "" {
		cfg.APIURL = apiURL
	}
	return saveCLIConfig(o.configPath, cfg)
}

func (o *rootOptions) clientWithoutToken() *apiClient {
	return newAPIClient(o.resolvedAPIURL(), "")
}

func (o *rootOptions) clientWithToken(token string) *apiClient {
	return newAPIClient(o.resolvedAPIURL(), token)
}

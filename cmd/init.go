package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/authproxy"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/template"
	"github.com/spf13/cobra"
)

func init() {
	var agent string
	prepare := &cobra.Command{Use: "init", Short: "Check the host and credentials, prepare the worker image and start proxies", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() == 0 {
			return fmt.Errorf("run init as the normal operator after host setup, so the proxy uses that account's credentials")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		projectCfg, err := loadProjectConfig(cwd)
		if err != nil {
			return err
		}
		applyRunConfigDefaults(runCmd)
		applyProjectRunConfig(runCmd, projectCfg)
		if _, err := parseAgentChoice(agentFlag); err != nil {
			return fmt.Errorf("configured run agent: %w", err)
		}
		if !cmd.Flags().Changed("agent") {
			// Match the configured default when it starts an agent. The
			// authenticated shell still prepares Codex as the baseline.
			if agentFlag == "codex" || agentFlag == "claude" {
				agent = agentFlag
			}
		}
		if agent != "codex" && agent != "claude" {
			return fmt.Errorf("choose --agent codex or claude")
		}
		report := runtimeReport(cmd.Context())
		if err := report.Write(cmd.OutOrStdout(), false); err != nil {
			return err
		}
		if err := report.Err(); err != nil {
			return err
		}
		if err := checkInitCredentials(cmd.Context(), agent); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Credentials configured (contents are never printed)."); err != nil {
			return err
		}
		m, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = m.Close() }()
		needs, reason, err := imageNeedsBuild(m, runOpts.Image, projectCfg)
		if err != nil {
			return err
		}
		if needs {
			name := template.MatchTag(runOpts.Image)
			if projectCfg != nil && projectCfg.Build.Dockerfile != "" && projectCfg.Build.Tag == runOpts.Image {
				if err = buildProjectImage(cmd.Context(), projectCfg, runOpts.Image); err != nil {
					return err
				}
			} else if projectCfg != nil && projectCfg.Build.Template != "" && projectCfg.Build.Tag == runOpts.Image {
				if err = buildFromTemplate(cmd.Context(), projectCfg.Build.Template, runOpts.Image); err != nil {
					return err
				}
			} else if name == "" {
				return fmt.Errorf("custom image %s is unavailable; build it first with oaiso build -f Dockerfile --tag %s", runOpts.Image, runOpts.Image)
			} else {
				if err = buildFromTemplate(cmd.Context(), name, runOpts.Image); err != nil {
					return err
				}
			}
			if _, err = fmt.Fprintln(cmd.OutOrStdout(), reason); err != nil {
				return err
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 40*time.Second)
		defer cancel()
		imageReport, err := engine.CheckImage(ctx, runOpts.Image, agent)
		if err != nil {
			return err
		}
		if err = imageReport.Write(cmd.OutOrStdout(), false); err != nil {
			return err
		}
		if err = imageReport.Err(); err != nil {
			return err
		}
		proxyAdminSecret = runProxyAdminSecret
		proxyAdminSecretFile = runProxyAdminSecretFile
		applyForwardPolicyDefaults(proxyRunCmd)
		if err = startProxyContainer(cmd.Context()); err != nil {
			return err
		}
		adminSecret, err := effectiveRunProxySecret()
		if err != nil {
			return err
		}
		proxy, err := connectProxyWithRetry(proxyAdminURL, proxyContainerURL, adminSecret, 30)
		if err != nil {
			return err
		}
		if _, err = reconcileProxyAuthState(cmd.Context(), proxy, false); err != nil {
			return err
		}
		launch := "oaiso codex"
		if agent == "claude" {
			launch = "oaiso run --agent claude"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Ready. From your project run: %s\nFor host edit watchers add --watch-polling to `oaiso run` (Chokidar/Watchpack).\n", launch)
		return err
	}}
	prepare.Flags().StringVar(&agent, "agent", "codex", "Prepare credentials and image checks for codex or claude (default: configured agent, otherwise codex)")
	registerCompletion(prepare, "agent", cobra.FixedCompletions([]cobra.Completion{"codex", "claude"}, cobra.ShellCompDirectiveNoFileComp))
	rootCmd.AddCommand(prepare)
}

func checkInitCredentials(ctx context.Context, agent string) error {
	if profile := selectedAuthProfile(); profile != "" {
		store, err := openAuthProfile(profile)
		if err != nil {
			return err
		}
		value, err := store.Load(ctx, profile)
		if err != nil {
			return err
		}
		if agent == "codex" && value.OpenAIAPIKey == "" && value.OpenAIOAuth == nil || agent == "claude" && value.AnthropicAPIKey == "" && value.AnthropicOAuth == nil {
			return fmt.Errorf("selected profile does not configure %s credentials", agent)
		}
		return nil
	}
	if agent == "codex" {
		info, err := authproxy.GetAuthInfo()
		if err != nil || !info.KeyConfigured {
			return fmt.Errorf("OpenAI credentials missing: run codex login, or set OPENAI_API_KEY and run oaiso auth set; then rerun init")
		}
	} else if agent == "claude" && !hostProxyAuthState().anthropicAvailable {
		return fmt.Errorf("anthropic credentials missing: run `claude auth login`, or set ANTHROPIC_API_KEY / CLAUDE_CODE_OAUTH_TOKEN; then rerun init --agent claude")
	}
	return nil
}

package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/pacificbelt30/codex-dock/internal/localkeyring"
	"github.com/pacificbelt30/codex-dock/internal/logincheck"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() { rootCmd.AddCommand(newLoginCommand()) }

func newLoginCommand() *cobra.Command {
	var agent, method, profile string
	var keyStdin, skipCheck, noSelect, encrypt, keyring, plaintext bool
	cmd := &cobra.Command{Use: "login", Short: "Log in with an independent profile and prepare authentication settings", Args: cobra.NoArgs}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent: codex or claude; prompt when omitted")
	cmd.Flags().StringVar(&method, "method", "", "Authentication: api-key or subscription; prompt when omitted")
	cmd.Flags().StringVar(&profile, "profile", "", "New profile name; existing profiles are never overwritten")
	cmd.Flags().BoolVar(&keyStdin, "key-stdin", false, "Read the API key explicitly from stdin")
	cmd.Flags().BoolVar(&skipCheck, "skip-check", false, "Save an API key without a network check, marked unverified")
	cmd.Flags().BoolVar(&noSelect, "no-select", false, "Save the new profile without changing global settings")
	cmd.Flags().BoolVar(&encrypt, "encrypt", false, "Encrypt with a product passphrase instead of the default keyring")
	cmd.Flags().BoolVar(&keyring, "keyring", true, "Encrypt using Secret Service (default; use auth keyring session on headless hosts)")
	cmd.Flags().BoolVar(&plaintext, "plaintext", false, "Explicitly save unencrypted credentials with mode 0600")
	cmd.MarkFlagsMutuallyExclusive("encrypt", "keyring", "plaintext")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if os.Geteuid() == 0 {
			return fmt.Errorf("run login as the normal operator")
		}
		if cmd.Flags().Changed("keyring") && !keyring {
			return fmt.Errorf("choose --encrypt or --plaintext explicitly instead of --keyring=false")
		}
		keyring = !encrypt && !plaintext
		input := cmd.InOrStdin()
		terminal := false
		var terminalInput *os.File
		if f, ok := input.(*os.File); ok {
			terminalInput = f
			terminal = term.IsTerminal(int(f.Fd()))
		}
		reader := bufio.NewReader(input)
		var err error
		if agent == "" && terminal {
			agent, err = loginChoice(reader, cmd.OutOrStdout(), "Agent", []string{"codex", "claude"})
			if err != nil {
				return err
			}
		}
		if agent != "codex" && agent != "claude" {
			return fmt.Errorf("choose --agent codex or claude (or run login in a terminal)")
		}
		if method == "" && terminal {
			method, err = loginChoice(reader, cmd.OutOrStdout(), "Authentication", []string{"subscription", "api-key"})
			if err != nil {
				return err
			}
		}
		if method != "api-key" && method != "subscription" {
			return fmt.Errorf("choose --method api-key or subscription")
		}
		if method == "subscription" && (keyStdin || skipCheck) {
			return fmt.Errorf("--key-stdin and --skip-check apply only to API keys")
		}
		if keyStdin && (!cmd.Flags().Changed("agent") || !cmd.Flags().Changed("method")) {
			return fmt.Errorf("--key-stdin requires explicit --agent and --method")
		}
		if profile == "" {
			var nonce [4]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				return err
			}
			profile = agent + "-" + hex.EncodeToString(nonce[:])
		}
		store, err := openAuthProfile(profile)
		if err != nil {
			return err
		}
		if _, err := store.Load(cmd.Context(), profile); !errors.Is(err, credentialstore.ErrNotFound) {
			if err != nil {
				return err
			}
			return fmt.Errorf("profile already exists; choose a new --profile name")
		}
		if keyring {
			if err := (localkeyring.SecretService{}).Probe(cmd.Context()); err != nil {
				return fmt.Errorf("keyring is not ready: %w; on headless hosts use oaiso auth keyring session", err)
			}
		}
		var pass []byte
		if encrypt {
			pass, err = readPassphrase(cmd, true)
			if err != nil {
				return err
			}
			defer clear(pass)
		}
		var ctx context.Context
		cancel := func() {}
		defer func() { cancel() }()
		var value credentialstore.Credentials
		verified := false
		if method == "api-key" {
			var data []byte
			if keyStdin {
				data, err = readProfileInput(cmd)
			} else if terminal {
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "API key: ")
				data, err = term.ReadPassword(int(terminalInput.Fd()))
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			} else {
				return fmt.Errorf("use a terminal or --key-stdin; host authentication is never imported")
			}
			if err != nil {
				return fmt.Errorf("reading API key failed")
			}
			defer clear(data)
			ctx, cancel = signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			key := strings.TrimSpace(string(data))
			provider := "openai"
			if agent == "claude" {
				provider = "anthropic"
				value.AnthropicAPIKey = key
			} else {
				value.OpenAIAPIKey = key
			}
			if !skipCheck {
				if err := logincheck.APIKey(ctx, provider, key); err != nil {
					return err
				}
				verified = true
			}
		} else {
			ctx, cancel = signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Starting a temporary Kata login VM (no workspace or host credentials).")
			value, err = subscriptionLogin(ctx, agent, reader, cmd.OutOrStdout())
			if err != nil {
				return err
			}
		}
		if err := createLoginProfile(ctx, profile, value, encrypt, keyring, pass); err != nil {
			return err
		}
		storage := "plaintext"
		if encrypt || keyring {
			storage = "encrypted"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Profile saved: %s (%s, 0600).\n", profile, storage)
		if verified {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Connection check: passed. Inference: not tested.")
		} else if method == "subscription" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Official CLI login: completed. Inference: not tested.")
		} else {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Connection check: skipped. Inference: not tested.")
		}
		if !noSelect {
			if err := selectLoginProfile(ctx, cmd.OutOrStdout(), profile); err != nil {
				return fmt.Errorf("profile %s is saved, but selecting it failed: %w", profile, err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Next: oaiso init --agent %s, then oaiso %s\n", agent, agent)
		} else {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Global settings were not changed (--no-select).")
		}
		return err
	}
	return cmd
}

func loginChoice(input *bufio.Reader, output io.Writer, title string, choices []string) (string, error) {
	_, _ = fmt.Fprintln(output, title+":")
	for i, choice := range choices {
		_, _ = fmt.Fprintf(output, "  %d. %s\n", i+1, choice)
	}
	_, _ = fmt.Fprint(output, "> ")
	line, err := input.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("login selection was canceled")
	}
	line = strings.TrimSpace(line)
	for i, choice := range choices {
		if line == fmt.Sprint(i+1) || line == choice {
			return choice, nil
		}
	}
	return "", fmt.Errorf("choose one of: %s", strings.Join(choices, ", "))
}

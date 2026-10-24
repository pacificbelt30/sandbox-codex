package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pacificbelt30/codex-dock/internal/localkeyring"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var authProfileName, authProfileDir string

func selectedAuthProfile() string {
	if flag := rootCmd.PersistentFlags().Lookup("auth-profile"); flag != nil && flag.Changed {
		return authProfileName
	}
	if authProfileName != "" {
		return authProfileName
	}
	return viper.GetString("auth_profile")
}

func authProfilesDirectory() (string, error) {
	directory := authProfileDir
	if directory == "" {
		directory = viper.GetString("auth_profile_dir")
	}
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(home, ".config", "codex-dock", "profiles")
	}
	store, err := credentialstore.NewFileStore(directory)
	if err != nil {
		return "", err
	}
	return store.Directory(), nil
}

// Each proxy mounts just this profile's directory, including its writable
// refresh state. It cannot read sibling profiles or any host CLI files.
func openAuthProfile(name string) (credentialstore.ProfileStore, error) {
	if err := credentialstore.ValidateProfileName(name); err != nil {
		return nil, err
	}
	directory, err := authProfilesDirectory()
	if err != nil {
		return nil, err
	}
	return credentialstore.OpenProfile(filepath.Join(directory, name), name)
}

func readProfileInput(cmd *cobra.Command) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), (1<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("reading credential input failed")
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("credential input exceeds 1 MiB")
	}
	return data, nil
}

func newAuthProfileCommand() *cobra.Command {
	parent := &cobra.Command{Use: "profile", Short: "Manage independent owner-only credential profiles"}
	var provider, format string
	var keyStdin, credentialsStdin bool
	set := &cobra.Command{Use: "set NAME", Short: "Create a profile with an API key from hidden TTY or explicit stdin", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openAuthProfile(args[0])
			if err != nil {
				return err
			}
			var data []byte
			if keyStdin {
				data, err = readProfileInput(cmd)
			} else if input, ok := cmd.InOrStdin().(*os.File); ok && term.IsTerminal(int(input.Fd())) {
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "API key: ")
				data, err = term.ReadPassword(int(input.Fd()))
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			} else {
				return fmt.Errorf("use a terminal or --key-stdin; host API-key environment variables are not imported")
			}
			if err != nil {
				return fmt.Errorf("reading API key failed")
			}
			key := strings.TrimSpace(string(data))
			if key == "" {
				return fmt.Errorf("API key is empty")
			}
			var value credentialstore.Credentials
			switch provider {
			case "openai":
				value.OpenAIAPIKey = key
			case "anthropic":
				value.AnthropicAPIKey = key
			default:
				return fmt.Errorf("provider must be openai or anthropic")
			}
			if err := store.Create(cmd.Context(), args[0], value); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Profile created (plaintext, 0600).")
			return err
		}}
	set.Flags().StringVar(&provider, "provider", "openai", "API provider: openai or anthropic")
	set.Flags().BoolVar(&keyStdin, "key-stdin", false, "Read the API key from stdin instead of hidden TTY input")
	importCmd := &cobra.Command{Use: "import NAME", Short: "Create a profile from explicit credential JSON on stdin", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !credentialsStdin {
				return fmt.Errorf("--credentials-stdin is required; no host login files are automatically imported")
			}
			store, err := openAuthProfile(args[0])
			if err != nil {
				return err
			}
			data, err := readProfileInput(cmd)
			if err != nil {
				return err
			}
			value, err := decodeProfileInput(data, format)
			if err != nil {
				return err
			}
			if err := store.Create(cmd.Context(), args[0], value); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Profile imported (plaintext, 0600).")
			return err
		}}
	importCmd.Flags().BoolVar(&credentialsStdin, "credentials-stdin", false, "Read credential JSON explicitly from stdin")
	importCmd.Flags().StringVar(&format, "format", "credentials", "Input format: credentials, codex, or claude")
	show := &cobra.Command{Use: "show NAME", Short: "Show profile metadata without credentials", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return showAuthProfile(cmd, args[0]) }}
	list := &cobra.Command{Use: "list", Short: "List independent profile names", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			directory, err := authProfilesDirectory()
			if err != nil {
				return err
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if entry.IsDir() && credentialstore.ValidateProfileName(entry.Name()) == nil {
					if err := showAuthProfile(cmd, entry.Name()); err != nil {
						if errors.Is(err, credentialstore.ErrNotFound) {
							continue
						}
						return err
					}
				}
			}
			return nil
		}}
	use := &cobra.Command{Use: "use NAME", Short: "Select a saved profile and prepare global auth settings", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openAuthProfile(args[0])
			if err != nil {
				return err
			}
			if _, encrypted := store.(*credentialstore.EncryptedStore); !encrypted {
				if _, err := store.Load(cmd.Context(), args[0]); err != nil {
					return err
				}
			}
			return selectLoginProfile(cmd.Context(), cmd.OutOrStdout(), args[0])
		}}
	parent.AddCommand(set, importCmd, show, list, use)
	addEncryptedProfileCommands(parent)
	return parent
}

func showAuthProfile(cmd *cobra.Command, name string) error {
	store, err := openAuthProfile(name)
	if err != nil {
		return err
	}
	if encrypted, ok := store.(*credentialstore.EncryptedStore); ok {
		info, err := encrypted.Info(cmd.Context())
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Profile: %s\nStorage: encrypted (%s, 0600)\nFormat: %s\nCredentials: locked; live connection not checked\n", name, info.Backend, info.Format)
		return err
	}
	v, err := store.Load(cmd.Context(), name)
	if err != nil {
		return err
	}
	mode := func(key string, oauth bool) string {
		if oauth {
			return "oauth"
		}
		if key != "" {
			return "api-key"
		}
		return "disabled"
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Profile: %s\nStorage: plaintext (0600)\nOpenAI: %s\nAnthropic: %s\n", name, mode(v.OpenAIAPIKey, v.OpenAIOAuth != nil), mode(v.AnthropicAPIKey, v.AnthropicOAuth != nil))
	return err
}

func decodeProfileInput(data []byte, format string) (credentialstore.Credentials, error) {
	var value credentialstore.Credentials
	switch format {
	case "credentials":
		if json.Unmarshal(data, &value) != nil {
			return value, fmt.Errorf("invalid credential JSON")
		}
	case "codex":
		var input struct {
			Tokens      *credentialstore.OpenAIOAuth `json:"tokens"`
			LastRefresh string                       `json:"last_refresh"`
		}
		if json.Unmarshal(data, &input) != nil {
			return value, fmt.Errorf("invalid Codex credential JSON")
		}
		if input.Tokens == nil {
			input.Tokens = &credentialstore.OpenAIOAuth{}
			if json.Unmarshal(data, input.Tokens) != nil {
				return value, fmt.Errorf("invalid Codex credential JSON")
			}
		}
		input.Tokens.LastRefresh = input.LastRefresh
		value.OpenAIOAuth = input.Tokens
	case "claude":
		var input struct {
			OAuth *struct {
				AccessToken      string   `json:"accessToken"`
				RefreshToken     string   `json:"refreshToken"`
				ExpiresAt        int64    `json:"expiresAt"`
				SubscriptionType string   `json:"subscriptionType"`
				Scopes           []string `json:"scopes"`
			} `json:"claudeAiOauth"`
		}
		if json.Unmarshal(data, &input) != nil || input.OAuth == nil {
			return value, fmt.Errorf("invalid Claude credential JSON")
		}
		v := input.OAuth
		value.AnthropicOAuth = &credentialstore.AnthropicOAuth{AccessToken: v.AccessToken, RefreshToken: v.RefreshToken, ExpiresAt: v.ExpiresAt, SubscriptionType: v.SubscriptionType, Scopes: v.Scopes}
	default:
		return value, fmt.Errorf("format must be credentials, codex or claude")
	}
	return value, nil
}

func profileProxyState(ctx context.Context, name string) (proxyAuthState, string, error) {
	if state, connection, ok := runningEncryptedProfile(ctx, name); ok {
		return state, connection, nil
	}
	store, err := unlockedAuthProfile(ctx, name)
	if err != nil {
		return proxyAuthState{}, "", err
	}
	v, err := store.Load(ctx, name)
	return proxyAuthState{openAIOAuth: v.OpenAIOAuth != nil, anthropicAvailable: v.AnthropicAPIKey != "" || v.AnthropicOAuth != nil, anthropicOAuth: v.AnthropicOAuth != nil}, v.ConnectionVersion, err
}

// A private file mode cannot protect credentials if their containing directory
// is explicitly shared with code running as the same UID in a worker.
func ensureProfilesOutsideWorkerShares(project, clone string, mounts []sandbox.MountSpec) error {
	if selectedAuthProfile() == "" {
		return nil
	}
	directory, err := authProfilesDirectory()
	if err != nil {
		return err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return err
	}
	protected := []string{directory}
	keyringPaths, err := localkeyring.PrivatePaths()
	if err != nil {
		return err
	}
	protected = append(protected, keyringPaths...)
	if runProxyAdminSecretFile != "" {
		path, err := filepath.Abs(runProxyAdminSecretFile)
		if err != nil {
			return err
		}
		path, err = resolveSharePath(path)
		if err != nil {
			return err
		}
		protected = append(protected, path)
	}
	shares := []string{project}
	if clone != "" {
		shares = append(shares, clone)
	}
	for _, m := range mounts {
		shares = append(shares, m.Source)
	}
	contains := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	for _, source := range shares {
		path, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		path, err = resolveSharePath(path)
		if err != nil {
			return err
		}
		for _, private := range protected {
			if contains(private, path) || contains(path, private) {
				return fmt.Errorf("credential profiles, keyring files/sockets and admin secrets must be outside the workspace, clone and additional mounts")
			}
		}
	}
	return nil
}

// Clone destinations can be absent while an existing ancestor is a symlink.
func resolveSharePath(path string) (string, error) {
	var missing []string
	for {
		existing, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				existing = filepath.Join(existing, missing[i])
			}
			return existing, nil
		}
		if !os.IsNotExist(err) || filepath.Dir(path) == path {
			return "", err
		}
		missing = append(missing, filepath.Base(path))
		path = filepath.Dir(path)
	}
}

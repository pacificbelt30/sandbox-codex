package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/localkeyring"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/pkg/cxd/credentialstore"
	"github.com/spf13/cobra"
)

func addEncryptedProfileCommands(parent *cobra.Command) {
	encrypt := &cobra.Command{Use: "encrypt NAME", Short: "Explicitly migrate a plaintext profile to passphrase encryption after draining workers", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		directory, err := authProfilesDirectory()
		if err != nil {
			return err
		}
		store, err := credentialstore.NewEncryptedStore(filepath.Join(directory, args[0]), args[0])
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		if err := prepareProfileKeyChange(cmd.Context(), args[0], store.Directory()); err != nil {
			return err
		}
		pass, err := readPassphrase(cmd, true)
		if err != nil {
			return err
		}
		defer clear(pass)
		if err = store.EncryptPlaintext(cmd.Context(), pass); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Profile encrypted (0600). Plaintext record removed after ciphertext verification; deleted filesystem blocks/backups are not securely erased.")
		return err
	}}
	passwd := &cobra.Command{Use: "passwd NAME", Short: "Change an encrypted profile passphrase after draining workers", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, err := unlockedAuthProfile(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		encrypted, ok := store.(*credentialstore.EncryptedStore)
		if !ok {
			return fmt.Errorf("profile is not encrypted")
		}
		if err = prepareProfileKeyChange(cmd.Context(), args[0], store.Directory()); err != nil {
			return err
		}
		pass, err := readNewPassphrase(cmd)
		if err != nil {
			return err
		}
		defer clear(pass)
		if err = encrypted.ChangePassphrase(cmd.Context(), pass); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Passphrase changed. Restart the authentication proxy with the new passphrase.")
		return err
	}}
	passwd.Flags().Int("new-passphrase-fd", -1, "Read the new passphrase from a separate descriptor >= 3")
	lock := &cobra.Command{Use: "lock NAME", Short: "Stop this profile's workers and authentication proxies; keep saved work and ciphertext", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openAuthProfile(args[0])
		if err != nil {
			return err
		}
		if _, ok := store.(*credentialstore.EncryptedStore); !ok {
			return fmt.Errorf("profile is not encrypted")
		}
		if err = lockProfile(cmd.Context(), args[0], store.Directory()); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Profile locked: affected workers and authentication proxies stopped; saved work and ciphertext retained.")
		return err
	}}
	parent.AddCommand(encrypt, passwd, lock)
}
func readNewPassphrase(cmd *cobra.Command) ([]byte, error) {
	if cmd.Flags().Changed("new-passphrase-fd") {
		fd, _ := cmd.Flags().GetInt("new-passphrase-fd")
		return readPassphraseFD(fd)
	}
	return readTerminalPassphrase(cmd, true)
}
func createLoginProfile(ctx context.Context, name string, value credentialstore.Credentials, encrypt, keyring bool, pass []byte) error {
	if !encrypt && !keyring {
		store, err := openAuthProfile(name)
		if err != nil {
			return err
		}
		return store.Create(ctx, name, value)
	}
	directory, err := authProfilesDirectory()
	if err != nil {
		return err
	}
	store, err := credentialstore.NewEncryptedStore(filepath.Join(directory, name), name)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	if keyring {
		return store.CreateWithKeyring(ctx, name, localkeyring.SecretService{}, value)
	}
	return store.CreateWithPassphrase(ctx, name, pass, value)
}

// Key changes refuse live workers. The explicit lock command is the operation
// that may interrupt work; migration/passphrase changes never do that silently.
func prepareProfileKeyChange(ctx context.Context, name, directory string) error {
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	records, err := c.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.Labels["codex-dock.managed"] == "true" && r.Labels["codex-dock.auth-profile"] == name && r.State == "running" {
			return fmt.Errorf("profile has live workers; save work and run oaiso auth profile lock %s first", name)
		}
	}
	for _, r := range records {
		v, err := c.ContainerInspect(ctx, r.ID)
		if err != nil {
			return err
		}
		if v.HostConfig != nil && hasProfileMount(v.HostConfig, directory) {
			if v.State == nil || v.State.Running {
				return fmt.Errorf("profile proxy is still active; stop it before changing keys")
			}
			if err = c.ContainerRemove(ctx, r.ID, container.RemoveOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}
func lockProfile(ctx context.Context, name, directory string) error {
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	records, err := c.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}
	// Resolve each proxy's private store before stopping workers. Profile names
	// alone are insufficient: two independent storage roots may both use "work".
	scopes := map[string]string{}
	var targets []string
	for _, r := range records {
		v, err := c.ContainerInspect(ctx, r.ID)
		if err != nil {
			return err
		}
		if v.HostConfig == nil {
			continue
		}
		for _, m := range v.HostConfig.Mounts {
			if m.Target != "/run/cxd-profile" || m.ReadOnly {
				continue
			}
			scopes[strings.TrimPrefix(v.Name, "/")] = m.Source
			if v.NetworkSettings != nil {
				for _, n := range v.NetworkSettings.Networks {
					if n != nil && n.IPAddress != "" {
						scopes[n.IPAddress] = m.Source
					}
				}
			}
			if m.Source == directory {
				targets = append(targets, r.ID)
			}
		}
	}
	manager, err := sandbox.NewManager(managementConfig())
	if err != nil {
		return err
	}
	defer func() { _ = manager.Close() }()
	var failures []error
	for _, r := range records {
		if r.Labels["codex-dock.managed"] != "true" || r.Labels["codex-dock.auth-profile"] != name || r.State != "running" {
			continue
		}
		selected, known := workerProfileScope(r.Labels, scopes, directory)
		if !known {
			failures = append(failures, fmt.Errorf("cannot verify worker profile scope; stop worker %s explicitly before locking", r.ID))
			continue
		}
		if selected {
			if err = manager.StopContext(ctx, r.ID, 5); err != nil {
				failures = append(failures, fmt.Errorf("stopping profile worker: %w", err))
			}
		}
	}
	// Stop even when a worker stop failed, so it cannot continue using the proxy.
	for _, id := range targets {
		if err = c.ContainerStop(ctx, id, container.StopOptions{}); err != nil {
			failures = append(failures, fmt.Errorf("stopping profile proxy: %w", err))
		}
	}

	return errors.Join(failures...)
}

func workerProfileScope(labels map[string]string, scopes map[string]string, directory string) (selected, known bool) {
	endpoint, err := url.Parse(labels["codex-dock.proxy-container-url"])
	if err != nil || endpoint.Hostname() == "" {
		return false, false
	}
	scope, known := scopes[endpoint.Hostname()]
	return scope == directory, known
}

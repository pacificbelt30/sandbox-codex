package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/pacificbelt30/codex-dock/internal/localkeyring"
	"github.com/spf13/cobra"
)

func init() { authCmd.AddCommand(newKeyringCommand()) }

func newKeyringCommand() *cobra.Command {
	parent := &cobra.Command{Use: "keyring", Short: "Check a Secret Service or open an isolated headless keyring session"}
	check := &cobra.Command{Use: "check", Args: cobra.NoArgs, Short: "Verify keyring store/read/delete with a disposable random item", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := (localkeyring.SecretService{}).Probe(cmd.Context()); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Keyring: ready (store/read/delete verified; no provider request).")
		return err
	}}
	session := &cobra.Command{Use: "session [-- COMMAND ARG...]", Short: "Unlock a private keyring for a command or interactive shell; no GUI required", RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" || os.Geteuid() == 0 {
			return fmt.Errorf("run the keyring session as a normal Linux user")
		}
		for _, name := range []string{"dbus-daemon", "gnome-keyring-daemon", "secret-tool"} {
			if _, err := exec.LookPath(name); err != nil {
				return fmt.Errorf("missing %s; run oaiso setup to prepare keyring dependencies", name)
			}
		}
		dataDir, err := localkeyring.DataDirectory()
		if err != nil {
			return err
		}
		lock, err := localkeyring.SessionFileLock(dataDir)
		if err != nil {
			return err
		}
		defer func() { _ = lock.Close() }()
		_, statErr := os.Stat(filepath.Join(dataDir, "keyrings", "login.keyring"))
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Unlock the Oaiso headless keyring (separate from the desktop login keyring).")
		pass, err := readPassphrase(cmd, os.IsNotExist(statErr))
		if err != nil {
			return err
		}
		defer clear(pass)

		argv := args
		if len(argv) == 0 {
			argv = []string{"/bin/bash"}
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer cancel()
		child := exec.CommandContext(ctx, argv[0], argv[1:]...)
		child.Stdin = cmd.InOrStdin()
		child.Stdout = cmd.OutOrStdout()
		child.Stderr = cmd.ErrOrStderr()
		return localkeyring.Session(ctx, dataDir, pass, child)
	}}

	parent.AddCommand(check, session)
	return parent
}

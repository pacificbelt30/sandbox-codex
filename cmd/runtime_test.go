package cmd

import (
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestRuntimeConfigPrecedence(t *testing.T) {
	prev := runOpts
	t.Cleanup(func() { runOpts = prev; viper.Reset() })
	viper.Reset()
	t.Setenv("CODEX_DOCK_RUNTIME", "kata")
	t.Setenv("CODEX_DOCK_KATA_RUNTIME", "kata-env")
	viper.Set("run.runtime", "runc")
	viper.Set("run.kata_runtime", "kata-config")
	c := &cobra.Command{Use: "run"}
	c.Flags().StringVar(&runOpts.Runtime, "runtime", sandbox.RuntimeKata, "")
	c.Flags().StringVar(&runOpts.KataRuntime, "kata-runtime", sandbox.DefaultKataRuntime, "")
	applyRuntimeDefaults(c)
	if runOpts.Runtime != "kata" || runOpts.KataRuntime != "kata-env" {
		t.Fatal("environment did not override config")
	}
	if err := c.Flags().Set("runtime", "runc"); err != nil {
		t.Fatal(err)
	}
	if err := c.Flags().Set("kata-runtime", "kata-flag"); err != nil {
		t.Fatal(err)
	}
	applyRuntimeDefaults(c)
	if runOpts.Runtime != "runc" || runOpts.KataRuntime != "kata-flag" {
		t.Fatal("flags did not override environment")
	}
}

func TestRunProxyAdminConfigPrecedence(t *testing.T) {
	oldURL, oldSecret, oldOpts := proxyAdminURL, runProxyAdminSecret, runOpts
	t.Cleanup(func() {
		proxyAdminURL, runProxyAdminSecret, runOpts = oldURL, oldSecret, oldOpts
		viper.Reset()
	})
	viper.Reset()
	viper.Set("proxy.admin_url", "http://127.0.0.1:19081")
	viper.Set("proxy.admin_secret", "config-secret")
	t.Setenv("CODEX_DOCK_PROXY_ADMIN_SECRET", "env-secret")
	c := &cobra.Command{Use: "run"}
	c.Flags().StringVar(&proxyAdminURL, "proxy-admin-url", "http://127.0.0.1:18081", "")
	c.Flags().StringVar(&runProxyAdminSecret, "proxy-admin-secret", "", "")
	applyRunConfigDefaults(c)
	if proxyAdminURL != "http://127.0.0.1:19081" || runProxyAdminSecret != "env-secret" {
		t.Fatal("run did not use the management proxy configuration")
	}
	if err := c.Flags().Set("proxy-admin-secret", "flag-secret"); err != nil {
		t.Fatal(err)
	}
	if err := c.Flags().Set("proxy-admin-url", "http://127.0.0.1:20081"); err != nil {
		t.Fatal(err)
	}
	applyRunConfigDefaults(c)
	if proxyAdminURL != "http://127.0.0.1:20081" || runProxyAdminSecret != "flag-secret" {
		t.Fatal("flags did not override the management proxy configuration")
	}
}

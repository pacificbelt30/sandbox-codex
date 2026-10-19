package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/policyevents"
	"github.com/pacificbelt30/codex-dock/internal/proxysettings"
	"github.com/pacificbelt30/codex-dock/internal/workeregress"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func configuredPolicy() proxysettings.Settings {
	return proxysettings.Settings{UpstreamProxy: viper.GetString("proxy.upstream_proxy"), CAFile: viper.GetString("proxy.ca_bundle"), AllowDomains: viper.GetStringSlice("proxy.forward_allow_domains"), DenyDomains: viper.GetStringSlice("proxy.forward_deny_domains"), AllowCIDRs: viper.GetStringSlice("proxy.forward_allow_cidrs"), DenyCIDRs: viper.GetStringSlice("proxy.forward_deny_cidrs"), AllowPorts: viper.GetIntSlice("proxy.forward_allow_ports"), DenyPorts: viper.GetIntSlice("proxy.forward_deny_ports")}
}

func newPolicyCommand() *cobra.Command {
	policy := &cobra.Command{Use: "policy", Short: "Inspect and explain immutable egress policies without changing running workers"}
	var worker, preset string
	var domains []string
	var asJSON, resolve bool
	policy.PersistentFlags().StringVar(&worker, "worker", "", "Inspect a managed worker's actual selected proxy")
	policy.PersistentFlags().StringVar(&preset, "preset", "", "Preview an opt-in preset for new workers")
	policy.PersistentFlags().StringArrayVar(&domains, "allow-domain", nil, "Narrow the host allowlist for this preview")
	policy.PersistentFlags().BoolVar(&asJSON, "json", false, "Print structured JSON")
	get := func(cmd *cobra.Command) (workeregress.Snapshot, error) {
		if worker != "" {
			if preset != "" || len(domains) > 0 {
				return workeregress.Snapshot{}, fmt.Errorf("worker inspection cannot override its policy")
			}
			return workeregress.Inspect(cmd.Context(), engine.Socket(), worker)
		}
		rules, offline, err := workeregress.ResolvePreset(configuredPolicy().Rules(), preset, domains)
		if err == nil {
			_, err = networkpolicy.Compile(rules)
		}
		return workeregress.Snapshot{Source: "configuration for NEW workers; not a live update", PolicyID: workeregress.ID(rules), GeneralEgress: !offline, Rules: rules}, err
	}
	for _, name := range []string{"show", "check"} {
		name := name
		policy.AddCommand(&cobra.Command{Use: name, Short: "Validate and display the selected policy", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := get(cmd)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(s)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\nPolicy: %s\nGeneral egress: %t\nPrivate destinations blocked: %t\nAllow domains: %v\nDeny domains: %v\nAllow ports: %v\nDeny ports: %v\nAllow IP/CIDRs: %v\nDeny IP/CIDRs: %v\nFixed AI routes use their own live-token authorization.\n", s.Source, s.PolicyID, s.GeneralEgress, !s.GeneralEgress || s.Rules.BlockPrivate, s.Rules.AllowDomains, s.Rules.DenyDomains, s.Rules.AllowPorts, s.Rules.DenyPorts, s.Rules.AllowCIDRs, s.Rules.DenyCIDRs)
			return err
		}})
	}
	explain := &cobra.Command{Use: "explain HOST[:PORT]", Short: "Explain a destination without opening an upstream connection", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, err := get(cmd)
		if err != nil {
			return err
		}
		if !s.GeneralEgress {
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"allowed": false, "code": "general_egress_disabled", "complete": true})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "DENY general_egress_disabled (fixed AI routes are independent)")
			return err
		}
		d, err := networkpolicy.Explain(cmd.Context(), s.Rules, args[0], resolve, nil)
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(d)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "allowed=%t code=%s stage=%s complete=%t addresses_checked=%d\n", d.Allowed, d.Code, d.Stage, d.Complete, d.AddressesChecked)
		return err
	}}
	explain.Flags().BoolVar(&resolve, "resolve", false, "Resolve and validate every DNS answer; never connect")
	policy.AddCommand(explain, &cobra.Command{Use: "presets", Short: "List optional policy presets", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(workeregress.Presets())
		}
		for _, p := range workeregress.Presets() {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", p.Name, p.Description); err != nil {
				return err
			}
		}
		return nil
	}})
	var follow bool
	events := &cobra.Command{Use: "events", Short: "Read retained denial events for --worker (shared proxy history)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if worker == "" {
			return fmt.Errorf("--worker is required")
		}
		return workeregress.Events(cmd.Context(), engine.Socket(), worker, follow, func(e policyevents.Event) error {
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(e)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s source=%s destination=%s reason=%s policy=%s\n", e.Time.Format("15:04:05"), e.Source, e.Destination, e.Code, e.PolicyID)
			return err
		})
	}}
	events.Flags().BoolVar(&follow, "follow", false, "Follow denial events until canceled")
	policy.AddCommand(events)
	return policy
}
func init() { rootCmd.AddCommand(newPolicyCommand()) }

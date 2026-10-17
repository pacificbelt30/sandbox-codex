package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

func newVMCommand() *cobra.Command {
	vm := &cobra.Command{Use: "vm", Short: "List, inspect and manage Kata worker VMs"}
	var all, jsonOutput bool
	var watch time.Duration
	ps := &cobra.Command{Use: "ps", Aliases: []string{"ls", "list"}, Short: "Reconcile worker records with actual Kata VM processes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if watch < 0 || watch > 0 && watch < time.Second {
			return fmt.Errorf("--watch must be zero or at least 1s")
		}
		if watch > 0 && jsonOutput {
			return fmt.Errorf("--watch and --json cannot be combined")
		}
		mgr, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = mgr.Close() }()
		for {
			ctx, cancel := context.WithTimeout(cmd.Context(), 35*time.Second)
			inventory, err := mgr.VMInventory(ctx)
			cancel()
			if err != nil {
				return err
			}
			inventory.VMs = visibleVMs(inventory.VMs, all)
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(inventory)
			}
			if err := printVMs(cmd, inventory); err != nil {
				return err
			}
			if watch == 0 {
				return nil
			}
			timer := time.NewTimer(watch)
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return cmd.Context().Err()
			case <-timer.C:
			}
		}
	}}
	ps.Flags().BoolVarP(&all, "all", "a", false, "Include stopped and created workers (orphans and inconsistencies are always visible)")
	ps.Flags().BoolVar(&jsonOutput, "json", false, "Print a JSON inventory with process identities and problems")
	ps.Flags().DurationVar(&watch, "watch", 0, "Repeat reconciliation, e.g. --watch 5s")
	inspect := &cobra.Command{Use: "inspect NAME|ID", Short: "Inspect a worker VM, its processes and resource envelope", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = mgr.Close() }()
		ctx, cancel := context.WithTimeout(cmd.Context(), 35*time.Second)
		defer cancel()
		row, err := mgr.VMInspect(ctx, args[0])
		if err != nil {
			return err
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(row)
	}}
	for _, action := range []string{"start", "stop", "rm"} {
		action := action
		var force bool
		timeout := 10
		c := &cobra.Command{Use: action + " NAME|ID...", Short: map[string]string{"start": "Start retained Kata workers", "stop": "Stop worker VMs; orphan recovery requires explicit --force if running", "rm": "Remove worker VMs or verified orphan envelopes"}[action], Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if timeout < 0 {
				return fmt.Errorf("timeout must be nonnegative")
			}
			mgr, err := sandbox.NewManager(managementConfig())
			if err != nil {
				return err
			}
			defer func() { _ = mgr.Close() }()
			var failures error
			for _, value := range args {
				ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
				switch action {
				case "start":
					err = mgr.VMStart(ctx, value)
				case "stop":
					err = mgr.VMStop(ctx, value, timeout, force)
				case "rm":
					err = mgr.VMRemove(ctx, value, force)
				}
				cancel()
				if err != nil {
					failures = errors.Join(failures, fmt.Errorf("%s %s: %w", action, value, err))
				} else if _, err = fmt.Fprintln(cmd.OutOrStdout(), value); err != nil {
					return err
				}
			}
			return failures
		}}
		if action != "start" {
			c.Flags().BoolVarP(&force, "force", "f", false, "Allow forced removal or stopping an attested running orphan VM")
		}
		if action == "stop" {
			c.Flags().IntVarP(&timeout, "timeout", "t", 10, "Seconds to wait for normal worker shutdown")
		}
		vm.AddCommand(c)
	}
	prune := &cobra.Command{Use: "prune", Short: "Recover empty orphan reservations; retain live VMs and stopped workers", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = mgr.Close() }()
		ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
		defer cancel()
		removed, cleanupErr := mgr.VMPrune(ctx)
		for _, id := range removed {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), id); err != nil {
				return errors.Join(cleanupErr, err)
			}
		}
		return cleanupErr
	}}
	vm.AddCommand(ps, inspect, prune)
	return vm
}
func visibleVMs(rows []kataguard.VM, all bool) []kataguard.VM {
	result := make([]kataguard.VM, 0, len(rows))
	for _, v := range rows {
		if all || v.State == "running" || v.State == "paused" || v.State == "unknown" || v.State == "inconsistent" || v.Orphan {
			result = append(result, v)
		}
	}
	return result
}
func printVMs(cmd *cobra.Command, inventory kataguard.Inventory) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tNAME\tSTATE\tTASK\tPIDS\tMEMORY\tPROBLEMS"); err != nil {
		return err
	}
	for _, v := range inventory.VMs {
		pids := make([]string, 0, len(v.Processes))
		for _, p := range v.Processes {
			pids = append(pids, fmt.Sprintf("%s:%d", p.Kind, p.PID))
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", shortVMID(v.ID), v.Name, v.State, v.EngineState, strings.Join(pids, ","), formatVMMemory(v.Resources["memory.current"]), strings.Join(v.Problems, "; ")); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	for _, problem := range inventory.Problems {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), problem); err != nil {
			return err
		}
	}
	return nil
}
func shortVMID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
func init() { rootCmd.AddCommand(newVMCommand()) }

func formatVMMemory(value string) string {
	bytes, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "-"
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
}

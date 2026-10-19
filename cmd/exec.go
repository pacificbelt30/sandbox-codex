package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

type workerExitError struct{ code int }

func (e workerExitError) Error() string {
	return fmt.Sprintf("worker command exited with status %d", e.code)
}
func (e workerExitError) ExitCode() int { return e.code }

const maxExecJSONStream = 8 << 20

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > b.limit {
		return 0, fmt.Errorf("exec JSON output exceeds %d bytes per stream", b.limit)
	}
	return b.Buffer.Write(data)
}

var execTimeout time.Duration
var execStdin, execJSON, execTTY bool
var execWorkdir string
var execEnv []string

var execCmd = &cobra.Command{
	Use:   "exec [flags] NAME -- COMMAND [ARG...]",
	Short: "Run a command in an existing worker",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		argv := execArguments(args)
		if len(argv) == 0 {
			return fmt.Errorf("exec requires a command after NAME")
		}
		mgr, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = mgr.Close() }()
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if execTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, execTimeout)
			defer cancel()
		}
		var in io.Reader
		if execStdin {
			in = cmd.InOrStdin()
		}
		out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
		stdoutBuf, stderrBuf := boundedOutput{limit: maxExecJSONStream}, boundedOutput{limit: maxExecJSONStream}
		if execJSON {
			out, errOut = &stdoutBuf, &stderrBuf
		}
		if execJSON && execTTY {
			return fmt.Errorf("--tty and --json cannot be combined")
		}
		code, runErr := mgr.ExecWithOptions(ctx, args[0], sandbox.ExecOptions{Argv: argv, Stdin: in, Stdout: out, Stderr: errOut, WorkingDir: execWorkdir, Env: execEnv, TTY: execTTY})
		if execJSON {
			result := struct {
				ExitCode int64  `json:"exit_code"`
				Stdout   string `json:"stdout"`
				Stderr   string `json:"stderr"`
				Error    string `json:"error,omitempty"`
			}{code, stdoutBuf.String(), stderrBuf.String(), ""}
			if runErr != nil {
				result.Error = runErr.Error()
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
				return err
			}
		}
		if errors.Is(runErr, context.DeadlineExceeded) {
			return workerExitError{124}
		}
		if errors.Is(runErr, context.Canceled) {
			return workerExitError{130}
		}
		if runErr != nil {
			return runErr
		}
		if code != 0 {
			return workerExitError{int(code)}
		}
		return nil
	},
}

func execArguments(args []string) []string {
	if len(args) < 2 {
		return nil
	}
	argv := args[1:]
	if argv[0] == "--" {
		argv = argv[1:]
	}
	return argv
}

var waitTimeout time.Duration
var waitJSON bool

var waitCmd = &cobra.Command{
	Use:   "wait [flags] NAME",
	Short: "Wait for a worker to exit and print its exit code",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = mgr.Close() }()
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if waitTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, waitTimeout)
			defer cancel()
		}
		code, err := mgr.WaitContext(ctx, args[0])
		if errors.Is(err, context.DeadlineExceeded) {
			return workerExitError{124}
		}
		if errors.Is(err, context.Canceled) {
			return workerExitError{130}
		}
		if err != nil {
			return err
		}
		if waitJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				ExitCode int64 `json:"exit_code"`
			}{code})
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), code)
		return err
	},
}

func init() {
	rootCmd.AddCommand(execCmd, waitCmd)
	execCmd.Flags().SetInterspersed(false)
	execCmd.Flags().BoolVarP(&execTTY, "tty", "t", false, "Allocate a guest PTY; output streams are combined")
	execCmd.Flags().StringVar(&execWorkdir, "workdir", "/workspace", "Guest working directory")
	execCmd.Flags().StringArrayVar(&execEnv, "env", nil, "Guest environment KEY=value (repeatable)")
	execCmd.Flags().DurationVar(&execTimeout, "timeout", 0, "Cancel the command after this duration (e.g. 30s)")
	execCmd.Flags().BoolVarP(&execStdin, "interactive", "i", false, "Forward standard input")
	execCmd.Flags().BoolVar(&execJSON, "json", false, "Return exit code, stdout and stderr as JSON")
	waitCmd.Flags().DurationVar(&waitTimeout, "timeout", 0, "Stop waiting after this duration")
	waitCmd.Flags().BoolVar(&waitJSON, "json", false, "Return exit code as JSON")
}

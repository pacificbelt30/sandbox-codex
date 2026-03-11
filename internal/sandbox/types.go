package sandbox

import (
	"io"
	"time"
)

// ApprovalMode controls how Codex CLI asks for approval when executing actions.
type ApprovalMode string

const (
	// ApprovalModeSuggest is the default interactive mode: Codex asks for
	// approval before every action.
	ApprovalModeSuggest ApprovalMode = "suggest"

	// ApprovalModeAutoEdit lets Codex apply file edits automatically but still
	// asks for approval before running shell commands
	// (maps to --ask-for-approval unless-allow-listed).
	ApprovalModeAutoEdit ApprovalMode = "auto-edit"

	// ApprovalModeFullAuto never asks for approval
	// (maps to --ask-for-approval never).
	ApprovalModeFullAuto ApprovalMode = "full-auto"

	// ApprovalModeDanger bypasses all approval prompts and Codex sandbox
	// restrictions (maps to --dangerously-bypass-approvals-and-sandbox).
	// Docker container isolation provides the safety boundary in this mode.
	ApprovalModeDanger ApprovalMode = "danger"
)

// ValidApprovalMode returns true if mode is one of the recognised values.
func ValidApprovalMode(mode ApprovalMode) bool {
	switch mode {
	case ApprovalModeSuggest, ApprovalModeAutoEdit, ApprovalModeFullAuto, ApprovalModeDanger:
		return true
	default:
		return false
	}
}

// RunOptions holds all options for starting a sandboxed container.
type RunOptions struct {
	Image         string
	Packages      []string
	PkgFile       string
	ProjectDir    string
	WorktreePath  string
	UseWorktree   bool
	Branch        string
	NewBranch     bool
	Name          string
	Task          string
	ApprovalMode  ApprovalMode
	Model         string
	ReadOnly      bool
	NoInternet    bool
	TokenTTL      int
	AgentsMD      string
	Detach        bool
	Parallel      int
	ShellMode     bool
	ContainerUser string // uid[:gid] to run as inside the container; empty = image default
}

// Worker represents a running or stopped codex-dock container.
type Worker struct {
	ID         string
	Name       string
	Status     string
	Image      string
	Branch     string
	Task       string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// LogOptions specifies options for fetching container logs.
type LogOptions struct {
	Name   string
	Tail   int
	Follow bool
	Output io.Writer // destination for log output; defaults to os.Stdout if nil
}

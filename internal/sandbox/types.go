package sandbox

import (
	"io"
	"time"
)

// RunOptions holds all options for starting a sandboxed container.
type RunOptions struct {
	Image        string
	Packages     []string
	PkgFile      string
	ProjectDir   string
	WorktreePath string
	UseWorktree  bool
	Branch       string
	NewBranch    bool
	Name         string
	Task         string
	FullAuto     bool
	Model        string
	ReadOnly     bool
	NoInternet   bool
	TokenTTL     int
	AgentsMD     string
	Detach       bool
	Parallel     int
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

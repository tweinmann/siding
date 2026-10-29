// Package fail holds the error type every siding command fails with: a stable code that agents
// may branch on, a message, and an optional hint on what to do next. Codes are part of the
// --json contract and are never renamed within an apiVersion.
package fail

import "fmt"

// Error is a failure with a stable code.
type Error struct {
	Code    string
	Message string
	Hint    string
}

func (e *Error) Error() string { return e.Message }

// New returns an Error with a formatted message.
func New(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithHint returns e with its hint set.
func (e *Error) WithHint(format string, args ...any) *Error {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}

// Error codes. The spec cli-contract lists them; add new ones there too.
const (
	// InvalidArguments: unknown command, flag or argument.
	InvalidArguments = "InvalidArguments"
	// NotAGitRepository: the current directory is not inside a git worktree.
	NotAGitRepository = "NotAGitRepository"
	// NoBranch: the worktree's HEAD is detached.
	NoBranch = "NoBranch"
	// ComposeFileNotFound: the worktree has no compose file.
	ComposeFileNotFound = "ComposeFileNotFound"
	// ComposeFailed: docker compose failed, or is missing or too old.
	ComposeFailed = "ComposeFailed"
	// DockerUnavailable: the Docker daemon cannot be reached.
	DockerUnavailable = "DockerUnavailable"
	// ProxyNotRunning: siding-proxy is not running.
	ProxyNotRunning = "ProxyNotRunning"
	// PortInUse: the proxy's port is held by another process.
	PortInUse = "PortInUse"
	// InvalidRoute: a dev.siding.route or dev.siding.port label cannot be used.
	InvalidRoute = "InvalidRoute"
	// AmbiguousPort: a routed service has several container ports and no dev.siding.port.
	AmbiguousPort = "AmbiguousPort"
	// SlugCollision: another branch or worktree already owns the Compose project.
	SlugCollision = "SlugCollision"
	// StackNotFound: the worktree has no stack.
	StackNotFound = "StackNotFound"
	// UnmergedBranch: removing data refused because the branch is not merged.
	UnmergedBranch = "UnmergedBranch"
	// DirtyWorktree: removing data refused because the worktree has changes.
	DirtyWorktree = "DirtyWorktree"
	// NoCapacity: the stack does not fit into the memory budget. The only code that exits 3.
	NoCapacity = "NoCapacity"
	// NotInteractive: a command would ask a question, but nobody can answer it.
	NotInteractive = "NotInteractive"
	// SnapshotNotFound: the repository has no snapshot of that name.
	SnapshotNotFound = "SnapshotNotFound"
	// SnapshotExists: the repository already has a snapshot of that name.
	SnapshotExists = "SnapshotExists"
	// EnvFileMissing: a linked worktree lacks an untracked env file the main worktree has.
	EnvFileMissing = "EnvFileMissing"
	// Internal: an unexpected error without a code of its own.
	Internal = "Internal"
)

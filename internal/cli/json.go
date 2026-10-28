package cli

import (
	"encoding/json"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// The documents --json prints are the contract with the agents that read them, so they are
// types of their own: an internal rename must not reach an agent. Within one apiVersion a field
// is never removed, renamed or given another meaning; new fields may be added. Empty fields are
// left out, never null. Every kind has a golden file in testdata/json.

// documentAPIVersion is the version of every document --json prints.
const documentAPIVersion = "siding.dev/v1alpha1"

const (
	kindAddress    = "Address"
	kindDown       = "Down"
	kindError      = "Error"
	kindGC         = "GC"
	kindLintReport = "LintReport"
	kindLogs       = "Logs"
	kindSetup      = "Setup"
	kindSkill      = "Skill"
	kindStack      = "Stack"
	kindStackList  = "StackList"
	kindStatus     = "Status"
	kindTeardown   = "Teardown"
	kindVersion    = "Version"
)

// jsonFlag is the name of the flag, looked up again by Execute to report an error as a document.
const jsonFlag = "json"

// header starts every document.
type header struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

func newHeader(kind string) header { return header{APIVersion: documentAPIVersion, Kind: kind} }

// registerJSON adds --json to a command.
func registerJSON(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, jsonFlag, false,
		"print one JSON document ("+documentAPIVersion+") instead of text, for scripts and agents")
}

// wantsJSON reports whether a command was run with --json. When parsing the flags failed, the
// flag holds no value yet, so the arguments themselves are checked.
func wantsJSON(cmd *cobra.Command, args []string) bool {
	if cmd != nil {
		if flag := cmd.Flags().Lookup(jsonFlag); flag != nil && flag.Changed {
			return flag.Value.String() == "true"
		}
	}
	if i := slices.Index(args, "--"); i >= 0 {
		args = args[:i]
	}
	return slices.ContainsFunc(args, func(a string) bool {
		return a == "--"+jsonFlag || strings.EqualFold(a, "--"+jsonFlag+"=true")
	})
}

func printJSON(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

// errorDocument is what a command run with --json prints when it fails.
type errorDocument struct {
	header
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type versionDocument struct {
	header
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// stackRef names a stack in other documents.
type stackRef struct {
	Project  string `json:"project"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	// URL is the stack's address: its host on the proxy's port.
	URL string `json:"url,omitempty"`
}

// setupDocument is what setup prints.
type setupDocument struct {
	header
	Container string `json:"container"`
	Network   string `json:"network"`
	Image     string `json:"image"`
	Port      int    `json:"port"`
	URL       string `json:"url"`
	// State is created, updated or unchanged.
	State string `json:"state"`
	// BudgetBytes is the memory budget for all stacks.
	BudgetBytes int64 `json:"budgetBytes"`
}

// stackDocument is what up prints without --wait.
type stackDocument struct {
	header
	Stack    stackRef         `json:"stack"`
	Services []serviceAddress `json:"services,omitempty"`
}

type serviceAddress struct {
	Name string `json:"name"`
	// URL is the service host.
	URL string `json:"url"`
	// Route is the path on the stack's host, if any.
	Route string `json:"route,omitempty"`
	Port  int    `json:"port"`
}

// statusDocument is the verdict of status and up --wait.
type statusDocument struct {
	header
	// Verdict is Ready, Starting, Failed or TimedOut.
	Verdict  string           `json:"verdict"`
	Stack    stackRef         `json:"stack"`
	Services []serviceStatus  `json:"services,omitempty"`
	Failure  *failureDocument `json:"failure,omitempty"`
}

type serviceStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Health string `json:"health,omitempty"`
	Route  string `json:"route,omitempty"`
	URL    string `json:"url,omitempty"`
	// Answering is set for services with a route path.
	Answering *bool `json:"answering,omitempty"`
	Ready     bool  `json:"ready"`
}

type failureDocument struct {
	Service string `json:"service"`
	// Reason is Exited, Unhealthy, Restarting, Missing, NotAnswering or NotReady.
	Reason string   `json:"reason"`
	Hint   string   `json:"hint"`
	Logs   []string `json:"logs,omitempty"`
}

// stackListDocument is what ps prints.
type stackListDocument struct {
	header
	Stacks      []stackEntry `json:"stacks,omitempty"`
	MemoryBytes uint64       `json:"memoryBytes,omitempty"`
	BudgetBytes int64        `json:"budgetBytes"`
}

type stackEntry struct {
	Project  string `json:"project"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	Machine  string `json:"machine,omitempty"`
	Agent    string `json:"agent,omitempty"`
	// State is running, partial or stopped.
	State           string `json:"state"`
	URL             string `json:"url"`
	MemoryBytes     uint64 `json:"memoryBytes,omitempty"`
	WorktreeMissing bool   `json:"worktreeMissing,omitempty"`
}

// logsDocument is what logs --json prints.
type logsDocument struct {
	header
	Stack    stackRef      `json:"stack"`
	Services []serviceLogs `json:"services,omitempty"`
}

type serviceLogs struct {
	Name  string   `json:"name"`
	Lines []string `json:"lines,omitempty"`
}

// addressDocument is what open --json prints.
type addressDocument struct {
	header
	URL     string `json:"url"`
	Service string `json:"service,omitempty"`
}

// downDocument is what down prints.
type downDocument struct {
	header
	Stack      stackRef `json:"stack"`
	DryRun     bool     `json:"dryRun,omitempty"`
	Containers []string `json:"containers,omitempty"`
	Networks   []string `json:"networks,omitempty"`
	Volumes    []string `json:"volumes,omitempty"`
}

// skillDocument is what skill --json prints.
type skillDocument struct {
	header
	Name string `json:"name"`
	Text string `json:"text"`
}

// gcDocument is what gc prints, and what the overview's /api/gc returns.
type gcDocument struct {
	header
	DryRun bool      `json:"dryRun,omitempty"`
	Stacks []gcStack `json:"stacks,omitempty"`
	Kept   []gcKept  `json:"kept,omitempty"`
}

type gcStack struct {
	Project  string `json:"project"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	// Reason is Merged or WorktreeGone.
	Reason      string   `json:"reason"`
	Containers  []string `json:"containers,omitempty"`
	Networks    []string `json:"networks,omitempty"`
	Volumes     []string `json:"volumes,omitempty"`
	KeptVolumes []string `json:"keptVolumes,omitempty"`
}

type gcKept struct {
	Project  string `json:"project"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	// Reason is Dirty, Unmerged or DefaultBranch.
	Reason string `json:"reason"`
}

// lintDocument is what lint --json prints.
type lintDocument struct {
	header
	File     string        `json:"file"`
	Findings []lintFinding `json:"findings,omitempty"`
}

type lintFinding struct {
	Rule string `json:"rule"`
	// Severity is error, warning or info.
	Severity string `json:"severity"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

// teardownDocument is what teardown prints.
type teardownDocument struct {
	header
	DryRun bool `json:"dryRun,omitempty"`
	// Container and Network are what teardown removes (or would).
	Container string `json:"container,omitempty"`
	Network   string `json:"network,omitempty"`
	// KeptNetwork is the proxy network when stacks still use it.
	KeptNetwork string `json:"keptNetwork,omitempty"`
	// Stacks keep running as plain Compose projects.
	Stacks []string `json:"stacks,omitempty"`
}

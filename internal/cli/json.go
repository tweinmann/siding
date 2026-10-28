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
	kindError   = "Error"
	kindVersion = "Version"
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
	return enc.Encode(doc)
}

// errorDocument is what a command run with --json prints when it fails.
type errorDocument struct {
	header
	Code    string `json:"code"`
	Message string `json:"message"`
}

type versionDocument struct {
	header
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

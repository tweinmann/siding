package cli

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// BuildInfo describes the running binary.
type BuildInfo struct {
	Version   string
	Commit    string
	GoVersion string
	Platform  string
}

// CurrentBuild returns the BuildInfo of this binary; the commit comes from the VCS stamp Go
// writes into it.
func CurrentBuild(version string) BuildInfo {
	info := BuildInfo{
		Version:   version,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				info.Commit = s.Value
			}
		}
	}
	return info
}

func newVersionCommand(build BuildInfo) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of siding",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if asJSON {
				return printJSON(cmd.OutOrStdout(), versionDocument{
					header:    newHeader(kindVersion),
					Version:   build.Version,
					Commit:    build.Commit,
					GoVersion: build.GoVersion,
					Platform:  build.Platform,
				})
			}
			line := "siding " + build.Version
			if build.Commit != "" {
				line += " (" + shortCommit(build.Commit) + ")"
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", line, build.GoVersion, build.Platform)
			return err
		},
	}
	registerJSON(cmd, &asJSON)
	return cmd
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

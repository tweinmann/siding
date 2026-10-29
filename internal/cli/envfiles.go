package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/git"
)

// missingEnvFiles returns the env files, relative to the worktree's top, that a linked worktree
// lacks and the main worktree has untracked: .env, which Compose reads for interpolation, and
// every required env_file of the compose file (spec stack-lifecycle, design D4). Compose itself
// only warns without .env and starts the stack with blank values.
func (a *app) missingEnvFiles(ctx context.Context, wt git.Worktree, file, project string) ([]string, error) {
	if !wt.Linked() {
		return nil, nil
	}
	candidates := []string{".env"}
	data, err := a.opts.Compose.UnresolvedConfig(ctx, compose.Invocation{Dir: wt.Top, Project: project, Files: []string{file}})
	if err != nil {
		return nil, err
	}
	envFiles, err := compose.EnvFiles(data)
	if err != nil {
		return nil, fail.New(fail.ComposeFailed, "%v", err)
	}
	for _, f := range envFiles {
		rel, err := filepath.Rel(wt.Top, f.Path)
		if f.Required && err == nil && !strings.HasPrefix(rel, "..") && rel != ".env" {
			candidates = append(candidates, rel)
		}
	}
	main := wt.MainTop()
	var missing []string
	for _, rel := range candidates {
		if exists, err := fileExists(filepath.Join(wt.Top, rel)); err != nil || exists {
			if err != nil {
				return nil, err
			}
			continue
		}
		if exists, err := fileExists(filepath.Join(main, rel)); err != nil || !exists {
			if err != nil {
				return nil, err
			}
			continue
		}
		tracked, err := a.opts.Git.Tracked(ctx, main, rel)
		if err != nil {
			return nil, err
		}
		if !tracked {
			missing = append(missing, rel)
		}
	}
	return missing, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

// envFileHint is the cp command per missing file, and what to do when the worktree needs none.
func envFileHint(wt git.Worktree, missing []string) string {
	var cps []string
	for _, rel := range missing {
		cps = append(cps, fmt.Sprintf("cp %s %s", filepath.Join(wt.MainTop(), rel), filepath.Join(wt.Top, rel)))
	}
	it, empty := "it", "an empty one"
	if len(missing) > 1 {
		it, empty = "them", "empty ones"
	}
	return fmt.Sprintf("copy %s from the main worktree: %s; or, if this worktree needs none, create %s: touch %s",
		it, strings.Join(cps, "; "), empty, strings.Join(missing, " "))
}

// checkEnvFiles fails with EnvFileMissing when the worktree lacks env files of the main worktree.
func (a *app) checkEnvFiles(ctx context.Context, wt git.Worktree, file, project string) error {
	missing, err := a.missingEnvFiles(ctx, wt, file, project)
	if err != nil || len(missing) == 0 {
		return err
	}
	return fail.New(fail.EnvFileMissing, "this worktree lacks %s, which the main worktree %s has and git does not track; without it the stack would start with blank values",
		strings.Join(missing, ", "), wt.MainTop()).WithHint("%s", envFileHint(wt, missing))
}

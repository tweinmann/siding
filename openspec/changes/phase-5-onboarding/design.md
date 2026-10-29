# Design

## Context

v0.4 (archived as `2026-10-29-phase-4-idle-and-wake`) works once siding is installed and the
project is prepared; the user journey reviewed with the maintainer on 2026-10-29 found the gaps
listed in proposal.md. The pieces this phase builds on: `cmd/siding` takes its version through
`-ldflags "-X main.version=…"`; `just build-darwin` cross-compiles statically; CI runs
everything inside the devcontainer image; `siding skill` embeds `internal/cli/skill/SKILL.md`;
`internal/lint` checks the normalised Compose config; `seedPlan` in `up` seeds a first `up`
only with `--seed`; `git.Worktree` knows the common git directory. Behaviour: see the delta
specs. Constraints from CLAUDE.md: the user's files are never edited, no real secrets in
output, standard building blocks over custom code.

Checked before this proposal with Compose 5.6.1: without `.env`, `docker compose config` only
warns that a variable is not set and uses a blank string, so a stack starts with empty values;
a missing `env_file` fails `config` with "env file … not found"; `config --no-env-resolution
--format json` lists every `env_file` with its `path` and `required: false` where set, even
when the files are missing.

## Goals / Non-Goals

**Goals:**
- A user who does not build siding can install it from a release in a minute.
- A new worktree either starts right (skill, permissions, env files, data) or says in one line
  what is missing and how to fix it.
- Nothing here writes a file of the user's but the one the user asks for with `--install`.

**Non-Goals:**
- Homebrew, notarised or signed binaries, an install script, Windows.
- Copying `.env` or secrets in any form; reading their values beyond what redaction and
  `LocalhostURL` already need.
- Creating worktrees, or integrating with a harness's own worktree feature.
- Rewriting `localhost` in the user's configuration.
- Presets for named harnesses.

## Decisions

### D1. Releases: `just dist` and `gh` (maintainer's decision)
`just dist <version>` runs `go build -trimpath -ldflags "-s -w -X main.version=<version>"` with
`CGO_ENABLED=0` for `darwin` and `linux` × `arm64` and `amd64` into `dist/` (git-ignored), and
writes `SHA256SUMS` with `sha256sum`, in the format `shasum -a 256 -c` reads on macOS. A
workflow `.github/workflows/release.yml` runs on tags `v*.*.*`: it runs `just test` and
`just dist "$GITHUB_REF_NAME"` in the devcontainer (as `ci.yml` does), then
`gh release create "$GITHUB_REF_NAME" dist/* --verify-tag --generate-notes` on the runner, with
`permissions: contents: write` for this job only. `just build-darwin` stays for development.
Alternatives, asked: goreleaser (archives, changelog, Homebrew later, but one more tool to pin in
the devcontainer, for four binaries). Homebrew and notarisation need an Apple developer account
and a tap repository and wait for users outside the maintainer's circle.

### D2. `siding skill --install` (maintainer's decisions: opt-in, `--dir`, snippets)
Without `--install`, `skill` prints as before. With it, `skill` writes
`<dir>/siding/SKILL.md` (directories created, an older copy replaced), where `<dir>` is `--dir`
(`~` expanded) or `.claude/skills` at the worktree's top, found with `git`; outside a worktree
and without `--dir` it fails with `NotAGitRepository`. It prints the path and, for the default
directory, the hint to commit the file, because a new worktree only has committed files; then
the two snippets of D3. With `--json`, the `Skill` document gains `path`, and the snippets go
to stderr, so stdout stays one document. `--dir` without `--install` is `InvalidArguments`.
The skill text names no harness (it already does not; a test keeps it so). Alternatives, asked:
only print (one more copy step for every repository); presets per harness (`--for codex`;
paths siding would have to track as they change); only an `AGENTS.md` snippet. A global install
is `--dir ~/.claude/skills`.

### D3. The snippets
The `AGENTS.md` section is a short Markdown section of four lines: run `siding up --wait --json`
after changes the running app depends on, act on the verdict, report `stack.url`, and leave
`down`, `gc` and data to the user; for the details it points to the skill. The permissions
are siding's reading commands, the only ones an agent needs: `up`, `status`, `logs`, `ps`, as
the Claude Code entry `{"permissions": {"allow": ["Bash(siding up:*)", "Bash(siding status:*)",
"Bash(siding logs:*)", "Bash(siding ps:*)"]}}` and as the plain list `siding up`,
`siding status`, `siding logs`, `siding ps` for other harnesses. siding writes neither:
what an agent may run without asking is the human's decision, as the guardrails put exposing
anything to the internet in the human's hands.

### D4. Env files of the main worktree (maintainer's decision: refuse, never copy)
`internal/git` gains `MainTop(w Worktree)`, the directory holding the common git directory when
it is named `.git` (the main worktree), and `Tracked(ctx, top, path)`, from
`git ls-files --error-unmatch`. In a linked worktree (its top differs from `MainTop`), before
`loadProject`, `up` runs `docker compose config --no-env-resolution --format json` and collects
the `env_file` paths without `required: false`, relative to the worktree, plus `.env`. Each that
is missing in the worktree, present in the main worktree and untracked there is a missing env
file; the first fails `up` with `EnvFileMissing`, the message naming all of them, the hint
`cp <main>/<path> <worktree>/<path>` per file, and `touch .env` when the worktree needs none.
A bare repository or a common directory not named `.git` has no main worktree: no check. `lint`
runs the same check and reports each as the warning `MissingEnvFile`; it then checks the rest
on the `--no-env-resolution` config, because the normal one fails without the file.
Alternatives, asked: only warn (`up` starts with blank values, which an agent would not notice
on stderr). Rejected: copying (`--copy-env`): secrets spread between directories, and an agent
could run it without the human knowing.

### D5. `LocalhostURL`
For every service and every variable of its normalised environment, a value matching
`(localhost|127\.0\.0\.1|\[::1\]):(\d+)` is a warning naming service, variable and the matched
`host:port`. The message and hint are built from those three only, so no other part of a value
(a password in a URL) reaches the text or the `LintReport`. A value without a port is not
reported: it is usually a cookie domain or a bind address, not a call to another service.
The hint: between services the service name (`http://api:3000`), in the browser a path on the
stack's address.

### D6. The snapshot `default` (maintainer's decision: by name)
`seedPlan` resolves, for a first `up` without `--seed`, the repository's snapshot `default`;
found, the plan seeds from it exactly as `--seed default`, and `up` prints `Seeding <project>
from the snapshot default (the repository's default; --seed none starts empty)`. `--seed none`
plans no snapshot. `snapshot save none` fails with `InvalidArguments`; `snapshot save default`
adds a line that new stacks start from it. The skill does not mention seeding: an agent's
first `up` gets the data without knowing. Alternatives, asked: a `--default` flag with a label
(Docker cannot change a volume's labels, so a snapshot could not become the default later, and
two defaults need a rule).

### D7. Codes, documents, tests
New error code `EnvFileMissing` (in `internal/fail` and the error-code table test); new lint
rules `MissingEnvFile` and `LocalhostURL`; `Skill` gains `path`. Golden files: `skill.json`
with a path, the `LintReport` with both rules, an `Error` for `EnvFileMissing`. The end-to-end
test adds: a linked worktree without `.env` refused, then started after the `cp` of the hint;
a first `up` seeded from `default` and one with `--seed none`. `just dist` is checked in CI by
the release workflow and by hand in task 7.

### D8. The README follows the user journey (maintainer's request)
"Using siding" becomes two parts. "Get started" is the journey in the order a user walks it:
install, once per machine, once per repository, per task with a worktree and an agent, keeping
an eye on it, after the merge; each step a few commands and sentences, linking to the details.
"Reference" keeps today's sections by topic. The tasks of this phase that document something
write into their step and their reference section. Alternative: keep the sections by command
and add the new ones (a first-time user has to find the order themselves).

## Risks / Trade-offs

- [Unsigned binaries trip Gatekeeper on macOS] → the README's `xattr` line; notarisation later.
- [A worktree that needs no `.env` although the main worktree has one] → the hint offers
  `touch .env`; one empty file, once per worktree.
- [The main worktree's `.env` is outdated for the branch] → siding only says what to copy; the
  user decides.
- [Worktrees with a custom layout (common directory not `.git`, bare repositories)] → no main
  worktree, no check: `up` behaves as in v0.4.
- [`LocalhostURL` false positives, e.g. a service's own health URL to itself] → a warning, not
  an error; the hint explains when it is fine.
- [A snapshot `default` seeds a stack someone wanted empty] → `up` says so on stderr; `--seed
  none`, or `down --volumes` and `up --seed none`.
- [`--no-env-resolution` changes in Compose] → pinned Compose in the devcontainer; a test against
  `docker compose config` in `just smoke-stacks`.

## Migration Plan

Nothing to migrate. Stacks, labels and documents of v0.4 stay valid. A user who has a snapshot
named `none` from v0.4 can still use and remove it; only new saves of that name are refused.
The first release is `v0.5.0`, tagged after this phase is approved.

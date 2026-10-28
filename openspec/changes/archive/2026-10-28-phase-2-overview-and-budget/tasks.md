# Tasks

## 1. Foundations

- [x] 1.1 Add `Stdin` and `IsTerminal` to `cli.Options` (wired in `cmd/siding/main.go`, fakes in the test env); verify `just test` passes and no package-level variable is a seam
- [x] 1.2 Add the error codes `NoCapacity` (exit 3 in `Execute`) and `NotInteractive` (D11); verify with the error-code table test and a test that `NoCapacity` exits 3 with an `Error` document
- [x] 1.3 Extend `internal/compose` with `container_name`, bind mounts, healthchecks, memory limits (`mem_limit`, `deploy.resources.limits.memory`), replicas (`deploy.replicas`, `scale`), and names and labels of top-level networks and volumes; verify with a second captured config in `internal/compose/testdata/` and table assertions

## 2. Merged and the git directory

- [x] 2.1 Extend `internal/git` with the shared merged check (ancestor of the default branch, or upstream `[gone]`), runnable with `--git-dir` for a missing worktree, and the gc conditions (not the default branch, default branch moved past the tip) (D7); verify with table tests against temporary repositories including a clone whose upstream branch was deleted and pruned, a fresh branch without own commits, and a deleted worktree
- [x] 2.2 Let `down --volumes` accept a branch whose upstream is gone; verify with a fake-based test and the end-to-end guard case
- [x] 2.3 Put `dev.siding.gitdir` on containers only (D8); verify with the override golden (`just golden`) and a test that networks and volumes do not carry it
- [x] 2.4 Add `dev.siding.gitdir` and `dev.siding.budget` to the label conventions in CLAUDE.md and `openspec/config.yaml`; verify `openspec validate --all --strict` passes

## 3. Memory budget

- [x] 3.1 Implement size parsing (`512m`, `12g`, bytes, `default`) and the estimate of a project (D5); verify with table tests including replicas and both limit forms
- [x] 3.2 Rework `setup` to always run Compose and compare container IDs (D6), and add `--budget` with the label carried forward; verify with fake-based tests for created, updated, unchanged, a budget change, `--budget default`, a v0.1 proxy, and the updated `setup.json` golden
- [x] 3.3 Make `up` refuse with `NoCapacity` (exit 3) when use of other stacks plus estimate exceeds the budget, and start anyway with `--force` and a warning; verify with fake-based tests for over, within, forced, and the stack not counting twice
- [x] 3.4 Show the budget in `ps` (total line, `budgetBytes` in `StackList`); verify with the updated text test and `stacklist.json` golden
- [x] 3.5 Document the budget in the README (`setup --budget`, the estimate, `mem_limit`, exit 3, `--force`); verify the commands run as written in the devcontainer

## 4. gc

- [x] 4.1 Implement the pure gc plan (D8): `Merged`, `WorktreeGone` with and without a merged branch, stacks without `dev.siding.gitdir`, kept stacks with `Dirty` or `Unmerged`, other machines ignored, the default branch never; verify with a table test per scenario of the spec `stack-gc`
- [x] 4.2 Implement `siding gc [--dry-run] [--yes] [--json]` with the question on a terminal and `NotInteractive` otherwise, removing by project name; verify with fake-based tests (answer y, answer n, no terminal, `--json` without `--yes`) and `gc.json` and `gc-dryrun.json` goldens
- [x] 4.3 Document `siding gc` in the README; verify the shown commands run as written

## 5. lint

- [x] 5.1 Implement the lint rules and severities of the spec `compose-lint` (D9); verify with a captured config that triggers every rule and one that triggers none
- [x] 5.2 Implement `siding lint [--file] [--json]` with exit 1 on errors and the `LintReport` document; verify with fake-based tests, a text test and a `lintreport.json` golden
- [x] 5.3 Document `siding lint` in the README with the rules in one table; verify the table lists every rule the code has

## 6. Overview

- [x] 6.1 Extend the proxy's compose file with the file provider on `~/.siding/proxy/<project>/routes` and the host alias decided by `setup` (D2: the network's gateway when it is a local address, else `host-gateway`); verify with fake-based tests for both cases and the proxy compose file content
- [x] 6.2 Implement the route file of `siding ui` (atomic write, removal on exit, replacing a stale one) and the listen addresses of D2; verify with unit tests in a temporary home
- [x] 6.3 Implement the server: the page (D3) with stacks, budget and gc candidates, `GET /api/stacks` and `GET /api/gc` returning the same documents as `ps --json` and `gc --dry-run --json`, 405 for other methods; verify with `httptest` tests and a golden of the rendered page with fixed data
- [x] 6.4 Implement `siding ui [--port]` (ProxyNotRunning without the route support, prints both addresses, Ctrl-C ends with 130 after removing the route); verify with a fake-based test that the route file exists while it runs and is gone after cancel
- [x] 6.5 Document `siding ui` in the README; verify the shown command runs as written in the devcontainer

## 7. Agent skill

- [x] 7.1 Teach exit 3 in the skill (stop and tell the user) and keep `--force`, `--volumes`, `gc` and `share` out of it; verify with the updated skill test and `just golden`

## 8. Devcontainer and docs

- [x] 8.1 Add `just build-darwin` (arm64 and amd64, `CGO_ENABLED=0`); verify both binaries are Mach-O for their architecture (`file bin/siding-darwin-*`)
- [x] 8.2 Make Docker-in-Docker the only devcontainer setup in CLAUDE.md, the README ("siding in a devcontainer" with its limits) and `openspec/config.yaml` (architecture and roadmap without Docker-outside-of-Docker, also in the draft `phase-5-remote-host` if it mentions it); verify `openspec validate --all --strict` passes
- [x] 8.3 Document in CLAUDE.md's safety rules that siding used on the host is product use and that `hack/nuke.sh` leaves it alone; verify `hack/nuke.sh` contains no name of a product object (`siding-proxy`, `siding`)

## 9. End-to-end and integration

- [x] 9.1 Extend `internal/e2e`: a budget too small refuses with exit 3 and `--force` starts; `gc --dry-run --json` lists a merged stack and keeps a dirty one, `gc --yes` removes the merged one with its volumes; `siding ui` answers through the test proxy at `siding.localhost` and the route is gone after cancel; verify with `just smoke-stacks` and that no `siding-test-*` object is left
- [x] 9.2 Run the flow by hand in the devcontainer: `setup --port 8080 --budget 2g`, two stacks, `ps`, `siding ui` in the host browser at `http://siding.localhost:8080`, `lint`, `gc`; record the result in the phase summary
- [x] 9.3 Run the flow on the Mac with `bin/siding-darwin-arm64` against Docker Desktop, including `siding ui` (the VM case of D2) and port 80; record the result and fix what differs
- [x] 9.4 Update the status in CLAUDE.md, the README and `openspec/config.yaml` to v0.2; verify `just test` passes

## 10. Teardown

- [x] 10.1 Implement `siding teardown [--dry-run] [--json]` (D12) with the `Teardown` document; add network removal by name when unused to `internal/docker`; verify with fake-based tests per scenario of the requirement "Teardown removes the proxy" and `teardown.json` and `teardown-dryrun.json` goldens
- [x] 10.2 Document `siding teardown` in the README (also in "Once per machine: the proxy") and add it to the end-to-end test (teardown with a running stack keeps the stack and the network, without one removes both); verify with `just smoke-stacks` and the README commands in the devcontainer

## Workflow follow-up

- Stop and show the result; wait for the maintainer's approval of Phase 2.
- `/opsx:archive` merges the deltas into `openspec/specs/`.

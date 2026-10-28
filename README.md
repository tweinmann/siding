# siding

siding gives every git worktree its own Docker Compose stack, at its own `*.localhost` address,
and tells you and your coding agents whether it actually runs and what it costs.

> **Pre-alpha.** Nothing here is usable yet: `siding version` is the only command. The plan is
> in [openspec/changes/](openspec/changes) (one change per phase), what siding does today in
> [openspec/specs/](openspec/specs).

## Developing siding

Everything runs inside the devcontainer, which has its own Docker daemon; nothing is installed
on your machine and siding's tests never touch your Docker containers.

1. Open the repository in VS Code and choose **Dev Containers: Reopen in Container** (needs
   Docker Desktop, OrbStack or Docker Engine).
2. In the container's terminal:

   | Command | Purpose |
   |---|---|
   | `just test` | formatting, vet, unit tests with `-race`, golangci-lint, OpenSpec validation (as in CI) |
   | `just golden` | rewrite golden files after an intended output change |
   | `just build` | build `bin/siding` |
   | `just smoke-docker` | Compose, `ports: !reset []` and Traefik routing against the container's own daemon |

To remove everything siding development left on your Docker host, run `hack/nuke.sh` in a
terminal on the host (`--dry-run` lists it first).

Working agreements, guardrails and safety rules are in [CLAUDE.md](CLAUDE.md).

## License

Apache-2.0, see [LICENSE](LICENSE).

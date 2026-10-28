# siding

siding gives every git worktree its own Docker Compose stack, at its own `*.localhost` address,
and tells you and your coding agents whether it actually runs and what it costs.

> **Pre-alpha (v0.1).** Stacks per worktree, routing through one local proxy and the verdict
> of `siding status` work; expect rough edges and changes before v1. There are no releases
> yet: build it from source (see below). The plan is in [openspec/changes/](openspec/changes)
> (one change per phase), what siding does today in [openspec/specs/](openspec/specs).

## Using siding

siding needs Docker with Compose 2.24 or later, and git. Your compose file stays as it is.

### Once per machine: the proxy

```sh
siding setup                 # Traefik as siding-proxy on 127.0.0.1:80
siding setup --port 8080     # if port 80 is taken, or inside siding's devcontainer
```

Every stack is reached through this one proxy at `http://<repo>.<branch>.localhost`. Browsers
and curl resolve `*.localhost` to your machine by themselves; for other tools add the host to
`/etc/hosts`. Running `siding setup` again changes nothing unless the port differs.

### In a worktree: start the stack

```sh
siding up                    # e.g. shop on feature/login: http://shop.feature-login.localhost/
siding up -- --build web     # arguments after -- go to docker compose up
siding up --wait             # and wait for the verdict, like siding status --wait
```

`up` runs your compose file as the Compose project `<repo>-<branch>` with one more file,
`.siding/override.yml`. That file resets every published port, so branches never fight over
ports, and puts siding's labels and routes on the services. `.siding/` ignores itself in git.

- A service labelled `dev.siding.route: "/"` is the stack's root, `dev.siding.route: "/api"`
  answers under that path (the path is passed on unchanged), `dev.siding.route: "false"`
  hides a service.
- Every service with one container port (from `ports` or `expose`) also answers at
  `http://<service>.<repo>.<branch>.localhost`. With several ports, `dev.siding.port: "3000"`
  names the HTTP one.
- Published ports are gone, also a database's. Reach it with
  `docker compose -p <repo>-<branch> exec db psql …`.

### Does it run?

```sh
siding status                # Ready, Starting, Failed or TimedOut, per service
siding status --wait         # until Ready or Failed, at most --timeout (3m)
siding ps                    # every stack on this machine: state, address, memory
```

`Ready` means every service with a healthcheck is healthy and every service with a route
answers through the proxy. A failure names the service, the reason, a hint and its last log
lines. Exit codes: 0 Ready, 1 Failed, 2 TimedOut or still starting. Every command prints one
JSON document with `--json`, for scripts and agents.

### Logs, address, and taking it down

```sh
siding logs -f api           # docker compose logs of the stack
siding open                  # the stack's address in the browser; siding open api for a service
siding down                  # remove containers and networks; volumes and data stay
siding down --volumes        # also the data; refused while the branch is unmerged or the
                             # worktree dirty, unless --force; --dry-run lists it first
```

### For coding agents

`siding skill` prints a Claude Code skill that tells an agent to run `siding up --wait --json`
after a change and to act on the verdict. siding writes no file itself; install it with

```sh
mkdir -p .claude/skills/siding && siding skill > .claude/skills/siding/SKILL.md
```

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
   | `just smoke-stacks` | siding's own commands end to end against the container's own daemon |

   To try siding by hand: `just build`, then `bin/siding setup --port 8080` (the port the
   devcontainer forwards), and `bin/siding up --wait` in a project; its address,
   `http://<repo>.<branch>.localhost:8080`, opens in the browser on your machine. Port 8080
   must be free on your machine: VS Code reports it when it is not, rather than forwarding to
   another port. Then free it, or change the port in `.devcontainer/devcontainer.json` and pass
   the same one to `siding setup --port`.

To remove everything siding development left on your Docker host, run `hack/nuke.sh` in a
terminal on the host (`--dry-run` lists it first).

Working agreements, guardrails and safety rules are in [CLAUDE.md](CLAUDE.md).

## License

Apache-2.0, see [LICENSE](LICENSE).

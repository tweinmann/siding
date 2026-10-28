---
name: siding
description: Start this worktree's Docker Compose stack with siding and wait for the verdict, to check that a change to the app actually runs. Use after changing code, Dockerfiles, compose files or configuration that the running app depends on, and before reporting such work as done.
---

# Check that the app runs with siding

siding runs this git worktree's compose file as its own stack, reachable through a local proxy
at `http://<repo>.<branch>.localhost`, and tells you whether it actually runs. Every command
below prints one JSON document with `--json`.

## After a change that affects the running app

Run:

```sh
siding up --wait --json
```

It starts or updates the stack and waits, three minutes at most, for one `Status` document.
Read its `verdict` and the exit code:

- **0, `Ready`**: every service runs, every healthcheck is healthy and every routed service
  answers through the proxy. Report the address in `stack.url` (and a service's `url` if it
  matters) together with your result.
- **1, `Failed`**: `failure` names the `service`, the `reason` (`Exited`, `Unhealthy`,
  `Restarting`, `Missing`), a `hint` and the last log lines of that service in `logs`. Fix the
  cause in that service, then run `siding up --wait --json` again.
- **2, `TimedOut`**: the stack was not ready in time. `failure` names the first service that
  was not ready: `NotAnswering` means it runs but does not answer through the proxy (often it
  listens on 127.0.0.1 instead of 0.0.0.0, or on another port than the one routed), `NotReady`
  means it is still starting. Read the hint and the logs; if it is only slow, run
  `siding status --wait --json --timeout 5m`.

An `Error` document (exit 1, `kind: Error`) means siding could not act at all; its `code`,
`message` and `hint` say why. `ProxyNotRunning`: ask the user to run `siding setup`.

## More detail

- `siding status --json`: the current verdict, without waiting (exit 2 while starting).
- `siding logs --json` or `siding logs --json <service> --tail 200`: the last log lines per
  service. Values from `.env` and the services' environment appear as `***`.
- `siding ps --json`: every stack on this machine, with state and memory.

## Rules

- Do not edit `.siding/override.yml`; siding rewrites it on every `up`. Change the compose file
  or the service labels `dev.siding.route` and `dev.siding.port` instead.
- Leave stopping and removing stacks to the user; `siding down` only removes containers, and
  removing data is the user's decision.
- Do not paste log lines into places outside this conversation.

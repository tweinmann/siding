# Spec Delta

## MODIFIED Requirements

### Requirement: Estimate of a stack
The estimate of a stack SHALL be the sum over its services, once per replica, of the learned
peak of the service plus a quarter, capped at its memory limit; without a learned peak, of the
memory limit (`deploy.resources.limits.memory`, else `mem_limit`); without either, 256 MiB.

#### Scenario: Limits and defaults
- **WHEN** a stack has `db` with `mem_limit: 1g`, `web` with no limit and `api` with `deploy.resources.limits.memory: 512m`, and siding has learned no peak
- **THEN** its estimate is 1 GiB + 256 MiB + 512 MiB

#### Scenario: A learned peak
- **WHEN** siding has seen `web` of the repository `shop` use at most 400 MiB, and `web` has no limit
- **THEN** `web` counts 500 MiB

#### Scenario: Capped by the limit
- **WHEN** `db` has `mem_limit: 1g` and a learned peak of 900 MiB
- **THEN** `db` counts 1 GiB, not 1125 MiB

## ADDED Requirements

### Requirement: Learned peaks
Whenever siding reads the memory of running containers (`ps`, `ui`, `status`, `up`), it SHALL
keep the highest use seen per repository and service in `~/.siding/peaks.json`. The file is a
losable cache: without it, or when it cannot be read, estimates fall back to limits and
defaults, and siding writes a new one.

#### Scenario: A peak is learned
- **WHEN** `siding ps` reads 400 MiB for `web` of `shop`, and later 300 MiB
- **THEN** `peaks.json` holds 400 MiB for `shop/web`

#### Scenario: Branches share what they learn
- **WHEN** siding learned 400 MiB for `web` from the stack of `main`
- **THEN** the estimate for a new stack of `feature/x` of `shop` uses it

#### Scenario: The cache is lost
- **WHEN** `~/.siding/peaks.json` is deleted or holds no valid JSON
- **THEN** siding works as without learned peaks and starts learning anew

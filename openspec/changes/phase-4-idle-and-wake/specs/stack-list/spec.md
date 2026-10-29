# Spec Delta

## MODIFIED Requirements

### Requirement: Stack state in the list
The state in `ps` SHALL be `running` when every container of the stack runs, `partial` when
some do, `idle` when the stack sleeps as `siding status` defines `Idle`, and `stopped` when
none runs otherwise. `ps` MUST NOT probe the proxy; the full verdict is `siding status`.

#### Scenario: A stopped container
- **WHEN** one of three containers of a stack has exited
- **THEN** `siding ps` shows the stack as `partial`

#### Scenario: A sleeping stack
- **WHEN** `shop-feature-x` was stopped after its idle duration
- **THEN** `siding ps` shows it as `idle` with 0 bytes of memory, and `siding ps --json` lists it with `state: idle`

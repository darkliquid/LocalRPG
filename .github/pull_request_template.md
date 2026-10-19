<!--
Describe the problem and the outcome, not the diff. A reviewer who has never seen
this part of the codebase should understand what changes for the user.
-->

## What this changes

## Why

## Tracking

<!-- The Beads id this work closes, for example LocalRPG-t7p.1. GitHub is not the
     issue tracker for this repository, so there is no issue to link. -->

- Bead:
- Spec and plan, if the change decides an interface, a behaviour, or a tradeoff:

## Checks

- [ ] `mise run test` passes
- [ ] `mise run lint` passes
- [ ] `go test ./pkg/gui -update-docs` run, if a guide, a config key, a provider, or a preset changed
- [ ] `go test ./pkg/gui -update-dto` run, if the wire contract changed
- [ ] User-facing documentation updated, if the behaviour changed
- [ ] No credential, key, or secret is added to the tree
- [ ] Screenshots checked for credentials, if any were added

## Notes for the reviewer

<!-- Anything a reader would otherwise have to reconstruct: a tradeoff taken, a
     behaviour deliberately left alone, a follow-up recorded rather than fixed. -->

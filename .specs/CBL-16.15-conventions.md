# CBL-16.15. cbl-kit: English comments and documentation

| Field | Value |
| :-- | :-- |
| Epic | E16, `Convention alignments` |
| Repository | `cbl-kit` |
| Size | S |
| Wave and parallel group | W0-CORR |
| Depends on | `CBL-16.1` (convention), `CBL-16.2` (pilot pattern) |
| Contract version | not applicable (no contract changes; exported API and runtime strings untouched) |
| Status | done, review closed |

## 1. Context and goal

Customer correction of 16.10.2026: comments and documentation are written in English in
all files. cbl-kit is a library vendored by every service; the task is documentation
only, with source compatibility preserved.

## 2. Scope

In scope: English translation of all comments and godoc docstrings in the library
(money, hmac, ginmw, inbox, outbox, grpcmw, storagex), migration SQL comments, all six
specs (CBL-1.1 - 1.6), AGENTS.md and README.md.

Out of scope: code, exported identifiers, error strings and log strings (executable
contracts; the tests grep them), Russian error-message test fixtures (they prove the
strings pass through unchanged), e2e stands (the library has none), other repositories.

## 3. Requirements

FR-1. Language: no Russian comments or docstrings remain; godoc docstrings keep the
entity-name first line and end with a period; AGENTS.md development rules state the
English documentation rule.

FR-2. Library note: AGENTS.md records that cbl-kit has no local e2e stand and no
PowerShell scripts; integration checks run with `make test-integration` here and in
every dependent service (whose e2e scenarios follow their `test/e2e/E2E.md`).

FR-3. Validation: build, vet, unit and integration tests and lint green (integration
tests run on real PostgreSQL and Kafka via testcontainers per the existing specs).

## 4. Plan

Parallel translation by file groups -> AGENTS.md updates -> full battery ->
commit `CBL-16.15: English comments and docs`.

## 5. Reviews and runs

Verified: build+vet green, unit and integration tests green, lint 0, em-dash check
clean; remaining Cyrillic is limited to runtime error strings (`inbox/processor.go`,
`grpcmw/interceptors.go`) and error-message test fixtures (`grpcmw/grpcmw_test.go`,
`ginmw/ginmw_test.go`); none are comments.

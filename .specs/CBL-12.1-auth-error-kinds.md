# CBL-12.1. Unauthenticated and permission_denied error kinds

| Field | Value |
| :-- | :-- |
| Epic | E12 (task CBL-12.1, the `cbl-kit` part) |
| Repository | `cbl-kit` |
| Size | S |
| Depends on | - |
| Status | done |

## 1. Context and goal

The back-office RBAC (CBL-12.1 in `cbl-backoffice`) needs first-class 401/403 error
classes: `grpcmw.Kind` had no unauthenticated/permission-denied kinds, so services would
have to map them locally, losing the uniform `AbortFromError` and gRPC `MapError`
behavior. This spec records the additive enum extension.

## 2. Requirements

FR-1. `grpcmw`: `KindUnauthenticated` (gRPC `codes.Unauthenticated`, name
`unauthenticated`) and `KindPermissionDenied` (gRPC `codes.PermissionDenied`, name
`permission_denied`) are appended to the enum (existing numeric values unchanged);
`MapError` hides internal details as before.

FR-2. `ginmw.AbortFromError`: `KindUnauthenticated` -> HTTP 401 code `unauthenticated`
title `Unauthorized`; `KindPermissionDenied` -> HTTP 403 code `permission_denied` title
`Forbidden` (a domain code still overrides the default).

FR-3. Unit tests cover both mappings in `grpcmw` and `ginmw`; the AGENTS.md command
snippet is updated to the actual lint image (`latest-alpine`).

## 3. Checks

`go build ./...`, `go vet ./...`, `go test ./...` green; no other package changes;
additive API (consumers recompile without edits).

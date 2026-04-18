# internal/org

The `ana org` verb tree: `list`, `show`, and the nested `members`/`roles`/`permissions` groups (each with a `list` leaf). Dispatch-only; callers adapt a transport client to `Deps.Unary`.

## Files

- `org.go` — `New`, `Deps`, and the service path prefix (`textql.rpc.public.auth.PublicAuthService` — org lookups live under the auth service).
- `list.go` — `PublicAuthService.ListOrganizations`.
- `show.go` — `PublicAuthService.GetOrganization`, keyed by the active profile's orgId.
- `members.go` — `ListOrganizationMembers` (requires an explicit `orgId` in the request payload — see commit `233eb82`).
- `roles.go` — `RBACService.ListRoles` scoped to the active org.
- `permissions.go` — `RBACService.ListPermissions` (readonly catalog).
- `org_test.go` — fake `Unary` records path + payload; asserts wire-level field names and orgId plumbing.

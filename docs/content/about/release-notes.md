---
title: Release Notes - What's New and What Changed
description: Notable additions, fixes, and breaking changes in each Olla release. Use this page as the source of truth for upgrade migrations and behaviour changes.
keywords: olla release notes, changelog, breaking changes, upgrade, migration
---

# Release Notes

This page tracks notable additions, fixes, and breaking changes in Olla. Each entry
is written for an operator upgrading from a prior release: what changed, why, and the
exact migration step if a behaviour change affects you.

Breaking changes are tagged **Breaking**. Additions and fixes are tagged **Added** or
**Fixed**. Quoted field names and config keys are the literal wire or YAML identifiers.

## Dashboard remediation release

### Breaking: userinfo URLs now fail startup

An endpoint URL with embedded `user:pass@host` is rejected at config load. Previously
such a URL loaded silently and the credentials flowed into every status/dashboard JSON
surface as the literal URL string. The endorsed credential path is the `auth:` block,
which is held as `json:"-"` fields on the internal endpoint record and never reaches the
status layer.

**Before:**

```yaml
- url: "http://user:pass@host:8080"
  name: "protected-backend"
  type: "openai-compatible"
```

**After:**

```yaml
- url: "http://host:8080"
  name: "protected-backend"
  type: "openai-compatible"
  auth:
    type: basic
    username: user
    password: pass
```

For secrets kept out of config, use the `username_file` / `password_file` variants (see
[Endpoint Authentication](../configuration/endpoint-auth.md)).

The boot error mirrors the rejected URL into the equivalent `auth:` block verbatim, so
the fix is a literal copy-paste. **Be aware the credentials will appear in the error
string**: do not paste the failed startup line into a chat, issue, or public log. The
credentials were already present in the rejected config, so the error leaks nothing the
file did not already contain - but the audience may differ.

### Breaking: zero-traffic status semantics changed

`/internal/status` no longer reports `"status": "critical"` on a fresh boot with all
endpoints healthy. With no proxy traffic, the system status derives from endpoint health
alone, `success_rate` reports `"N/A"`, and a new always-present boolean
`system.has_traffic` lets clients branch on the no-traffic state without parsing the
success-rate string. Previously a healthy fresh boot fell through a `< 90.0` success-rate
threshold and reported `critical`, which coupled with the dashboard produced a misleading
red status on first start.

### Breaking: endpoint `id` derivation changed

The `id` values surfaced on `/internal/status`, `/internal/status/endpoints`, and the
per-model `endpoint_ids` on `/internal/status/models` are now derived from the sanitised
URL (scheme+host+port+path) with positional disambiguation for siblings that share a
sanitised form. Credential rotation no longer changes the ID, because userinfo, query,
and fragment are stripped before hashing. **Bookmarked dashboard deep-links to a specific
endpoint row will change once after upgrading**, then stay stable. The IDs are base36
FNV-1a, identical across all three status payloads for the same endpoint from the same
repository snapshot.

### Added: weak ETags on status JSON

The three status JSON routes now emit a **weak** `ETag` of the form `W/"<base36>"` over
their stable fields. `If-None-Match` uses weak comparison, so clients that echo the
`ETag` verbatim are unaffected. Clients doing strong-only comparison should allow weak
matches. See [Conditional requests and compression](../api-reference/system.md#conditional-requests-and-compression)
for the full contract.

### Fixed: proxy access logging restored

Successful proxy requests log at `Info` again in the access log. A regression had
demoted them to `Debug`, which silently broke the "log all requests" expectation the
security practices document promises. Only `/internal/` GET/HEAD polling that returns
`2xx` or `304` stays at `Debug` so an open dashboard tab does not flood the log; 4xx,
5xx, and any non-GET/HEAD method under `/internal/` continue to log at `Info`.

### Added: unbuilt-dashboard warning at startup

A binary built without `make build-web` (e.g. via `go install` or a plain `go build`)
now logs a clear startup line and serves `503` at `/internal/ui/` with a body naming the
fix. Previously such a binary served a silent placeholder. See
[Admin Dashboard: building the frontend](../configuration/dashboard.md#building-the-frontend).

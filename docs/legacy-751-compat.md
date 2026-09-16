# SAP NetWeaver 7.51 compatibility facade

SAP NetWeaver 7.51 does not expose the ADT resources used by VSP's source-based
DDIC table editor and text-pool editor (both shipped with 7.52 SP00). The
standard repository interfaces do exist, but their raw interfaces are too broad
to expose as a generic MCP tool.

`abap/src/zvsp_compat/` supplies a deliberately narrow RFC facade:

- `TEXTPOOL_GET` / `TEXTPOOL_SET` delegate to `READ TEXTPOOL` and
  `RPY_TEXTELEMENTS_INSERT`.
- `TABLE_CREATE` delegates to `RPY_TABLE_INSERT` and then checks activation
  through `DDIF_TABL_ACTIVATE`.

The facade has a single function module, `ZVSP_COMPAT_751`, in function group
`ZVSP_COMPAT`. Its source and exact SE37 interface are documented in its
[deployment README](../abap/src/zvsp_compat/README.md).

## How VSP routes around the gaps

VSP decides per feature which route to take. Nothing has to be configured:
when the facade is not deployed, or no RFC connection is configured, the
compat routes are simply absent and the original errors come back unchanged.

| VSP feature | 7.52+ | 7.51 (facade deployed) | 7.51 (no facade) |
|---|---|---|---|
| `CreateTable` | ADT `/ddic/tables` (create → DDL → activate) | ADT answers 404 → same tool drives `TABLE_CREATE` over classic RFC | 404 error naming the 7.52 boundary and the facade |
| `TextElements` read | ADT `/textelements/.../source/<kind>` | one object-level probe (cached per client) routes reads to `TEXTPOOL_GET` | reads no longer return a silent empty pool; they fail with the boundary explanation |
| `SetTextElements` | lock → PUT → activate | probe routes the write to `TEXTPOOL_GET` → merge → `TEXTPOOL_SET` (full-pool overwrite) | error at the lock step, as before |
| Class text symbols | ADT resource | not covered: classic `READ TEXTPOOL` has no class equivalent; use SE24/SE80 | same |

Trigger conditions are deliberately narrow:

- Table creation falls back only when the create POST itself answers **404** —
  the resource does not exist on this release. Any other status keeps the
  original semantics. When the facade call also fails, both causes travel in
  the one error.
- Text-pool routing probes the object-level `textelements` resource once per
  client. A 404 there is only read as "this release lacks the resource" after
  the host object answered 200; a 404 on the host object is a missing program
  and is reported as such.
- The write path re-reads the pool before overwriting (the counterpart of the
  ADT path's read-under-lock), so entries nobody touched survive the
  full-pool overwrite — including entries of kinds this facade does not
  maintain, such as classic list headings (H).

Go side: the `CompatFallback` interface lives in `pkg/adt/compat751.go`, the
RFC driver in `pkg/saprfc/compat751.go`, and `internal/mcp` wires the shared
RFC connection pool to it at server start.

## What else 7.51 does not have — and what that means here

Inventory of VSP's create/write surface against a 7.51 backend:

- **RAP stack (`BDEF`, `SRVD`, `SRVB`, IAM apps)** — does not exist on 7.51 in
  any form; there is no classic equivalent to bridge to. These keep failing
  with their 404 and that is the correct answer, not a gap.
- **Table source editing** (`/ddic/tables/{name}/source/main`) — also a
  7.52+ resource, so *editing* an existing table's definition has no facade
  route yet. A `TABLE_UPDATE` operation (via `DDIF_TABL_PUT`) is the natural
  extension if it is needed.
- **Data element / domain creation** — not yet implemented on main at all
  (issue #109 / PR #192). Once it lands, its ADT resources need a real-7.51
  check; if they are missing the same facade pattern extends naturally
  (`DDIF_DTEL_PUT` / `DDIF_DOMA_PUT`).
- **Everything else VSP creates** (programs, includes, classes, interfaces,
  function groups/modules, packages, DDL sources) rides ADT resources that
  7.51 has; no compat route involved.
- Reads that the text-pool route depends on: master language and package
  lookups use the free-SQL preview resource, which predates 7.52 and is
  present on 7.51.

## Safety model

The facade accepts only names beginning with `Z` or `Y`. A non-local package
requires a supplied transport request; it preserves the standard SAP
authorization checks and never suppresses the activation authorization check
(`AUTH_CHK = 'X'`). It intentionally does not accept an arbitrary function
name.

For text pools, the client reads the full current pool, merges the desired
changes, and then writes the full merged `TEXTPOOL` table. The setter is not a
patch API, and VSP enforces that on its side (`compatPoolApply`).

For tables, activation is a separate operation. If it fails (`E_RC = 12` with
`E_ACTIVATION_RC > 4`), an inactive DDIC definition is left behind; inspect it
and the SAP activation log in SE11 — nothing deletes it automatically.

## Validation status

The Go side is covered by unit tests: fallback triggers only on 404, field
translation (built-ins, aliases, data-element fallback, client field
prepending), full-pool merge semantics, and result/error interpretation.

The ABAP module itself has not been deployed to a real 7.51 system yet. A
previous attempt via the controlled creation planner was automatically
compensated (function group and module deleted) because that planner cannot
materialize function-module interface parameters; the recommended deployment
route is now VSP's `CreateObject` workflow with `rfc_enabled=true` and the
signed source (see the facade README) — the ADT model defines a function
module's interface in its source, so no hand-built SE37 shell is needed.
Then verify with `rfc describe ZVSP_COMPAT_751`, and exercise the facade with
a disposable `Z/Y` program and table in a dedicated DEV transport before
relying on it.

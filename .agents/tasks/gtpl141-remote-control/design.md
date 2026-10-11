# GTPL-141 Remote-Control Integration — Requirements & Technical Design

Status: initial authoring (no prior design-review found).
Scope: two sibling repositories, edited in place (no git worktree).

- Backend (Go, gorilla/mux): `/Users/shubham/Desktop/project/grain_backend` — VPS-hosted Grain Technik API.
- Frontend (Next.js 16 / React 19 / TanStack Query / Radix + Tailwind): `/Users/shubham/Desktop/project/new_plc_software` — Vercel-hosted dashboard.

This document is the contract for the implementation phase. The technology stack
named here is locked once approved. Backend route paths named in §B2 are final;
the frontend (§C) must match them exactly.

---

## PART A — REQUIREMENTS

### A.1 Summary

Add a remote-control panel for the single machine **GTPL-141** that lets an
authenticated, authorized Grain dashboard operator read live control state and
submit a bounded set of PLC settings and momentary pulses. The browser never
holds the upstream credential: the Grain backend adds a new server-side adapter
that holds the integration bearer token (from server-only env), authorizes the
caller against the existing machine-scope machinery, forwards a hardcoded set of
GTPL-141-only requests to the upstream HVAC API (`primeosys.com/hvac-dashboard/api`),
and passes sanitized responses back. No real PLC write path is created, extended,
or simulated by this work beyond proxying the already-existing upstream command API.

### A.2 Functional requirements

**Backend adapter**

- FR-B1 Read `GTPL141_HVAC_BASE_URL` (default `https://primeosys.com/hvac-dashboard/api`)
  and `GTPL141_HVAC_TOKEN` from env via the existing `getEnv` helper. When the
  token is empty, every adapter route returns `503 integration not configured`
  without contacting upstream.
- FR-B2 Expose seven GTPL-141-only routes on the authenticated `protected`
  subrouter (final paths in §B2). The upstream unit (`unit-141`) and upstream
  paths are hardcoded; no unit, URL, or path segment is ever taken from the
  browser.
- FR-B3 Every route requires a valid Grain session (already enforced by
  `AuthenticateToken`) **and** confirms the caller is authorized for the physical
  table `GTPL_141_GT_140E_S7_1200` before any upstream call. Fleet-access accounts
  pass. Denials reuse the codebase's opaque authorization wording.
- FR-B4 State-changing POST routes additionally require that, when an `Origin`
  header is present, it passes `middleware.IsOriginAllowed`; a disallowed origin
  is rejected before any upstream call.
- FR-B5 A dedicated upstream `*http.Client` (15s timeout, TLS verification on, no
  automatic redirect following) builds a fresh request per call with server-side
  headers only (`Authorization: Bearer <token>`, `Accept: application/json`, and
  `Content-Type: application/json` on POST). No incoming browser header is
  forwarded. Upstream status codes and bodies pass through, sanitized to never
  leak the token, upstream `Set-Cookie`, or internal config. Upstream 401 surfaces
  as a clear integration-auth error with no credential fallback.
- FR-B6 `POST .../commands` accepts only the documented fields
  (`request_id`, `tag`, `value`, `expected_value`, `mode`, `reason`) and rejects
  unknown fields; enforces the writable-tag allowlist (§B6) and value typing;
  rejects the two hidden tags outright; supplies a default/derived `reason` when
  none is given; never auto-writes a complementary boolean bit.
- FR-B7 Each command POST writes a Grain-side audit log line linking the local
  operator, request UUID, tag, requested/expected values, timestamps, and upstream
  outcome. The upstream actor is documented as the shared integration identity.

**Frontend panel**

- FR-F1 Render the controls panel **only** for the exact GTPL-141 machine identity
  (§C.2), using the existing `api()` client (credentialed) against the new backend
  routes. No token, no direct `primeosys` call, no new chatbot.
- FR-F2 Three Start/Stop pulse groups and nine editable maintained settings, built
  from the existing Radix/shadcn component library and design tokens.
- FR-F3 TanStack Query polling only while the panel is mounted/visible, with no
  overlapping requests: latest ~6s, capabilities ~5s, commands ~3s; a 1s ticker
  recomputes freshness so a stalled poll cannot leave controls enabled.
- FR-F4 Enable/disable gating per §C.6; block all new actions while any command is
  in flight or any `reset_required` is set; one action at a time.
- FR-F5 Maintained-setting submit captures current readback as `expected_value`,
  shows selected-actual vs requested, requires a deliberate confirm, uses a stable
  per-action UUID reused across retries, and surfaces HMI-conflicts instead of
  silently resubmitting.
- FR-F6 Pulses send a single `{value:true, expected_value:false}` command; no false
  follow-up, no client-side 100ms timer.
- FR-F7 Full command-status handling (§C.9) including `simulated` never shown as a
  live success; cancel only `queued`; `uncertain` review flow with three outcomes
  and a separate inspection checkbox.
- FR-F8 HTTP-status-specific error handling (§C.10) with bounded timeouts.
- FR-F9 Unmount/close only stops UI polling; it never mutates PLC state.
- FR-F10 Every new user-facing string has real translations in `en`, `de`, and
  `th` so `npm run build` passes.

### A.3 Non-functional requirements

- NFR-1 No regression to any other machine, route, or existing dashboard behavior.
- NFR-2 Secret handling per the project rules: token only in backend server env;
  only blank/example entries committed; never in frontend, logs, fixtures, or
  bundle.
- NFR-3 Backend compiles with `go build ./...` and existing patterns (handler
  conventions, authz machinery, config loading, response shapes) are reused rather
  than reinvented.
- NFR-4 Frontend builds green: `node scripts/check-translations.mjs && next build`.

### A.4 Acceptance criteria (testable)

1. With `GTPL141_HVAC_TOKEN` unset, each of the seven routes returns HTTP 503 with
   `{"success":false,"message":"GTPL-141 remote control is not configured."}` and
   makes no outbound request (verifiable: no upstream client call; unit test with a
   stub client asserts zero calls).
2. A session whose authorized tables exclude `GTPL_141_GT_140E_S7_1200` (and lacks
   fleet access) receives HTTP 403 with the existing opaque wording on all seven
   routes; the response never names GTPL-141 or any other machine.
3. A fleet-access account and an account whose grant includes GTPL-141 both reach
   the upstream call path (verifiable with a stubbed upstream returning 200).
4. `GET /api/controls/gtpl141/history?minutes=1&limit=5` forwards `minutes=5` and
   `limit=20` (clamped to the documented floors); `minutes=99999&limit=99999`
   forwards `minutes=1440` and `limit=1000`.
5. `POST /api/controls/gtpl141/commands` with an unknown field (e.g. `foo`) returns
   HTTP 422 and does not call upstream.
6. The same POST with `tag:"Manual_EN"` or `tag:"Auto_aeration_disable"` returns
   HTTP 422 (hidden tag) and does not call upstream.
7. A pulse command (`tag:"Auto_start", value:true, expected_value:false`) is
   forwarded unchanged; no second command with `value:false` is ever emitted by the
   backend.
8. Type validation: `Delta_T` (REAL) with `value:"x"` → 422; `Duration_set_aeration`
   (INT) with `value:1.5` or `value:40000` → 422; `Auto_EN` (BOOL) with `value:1` →
   422 (must be JSON `true`/`false`).
9. A command POST with no `reason` is forwarded with
   `reason:"Grain Technik dashboard command; operator=<UserID>"`; the browser cannot
   override the operator identity.
10. Changing `Auto_EN` emits exactly one tag write; no `Manual_EN` write is appended.
11. An upstream 401 is surfaced to the browser as an integration-auth error
    (distinct message) and the backend logs it without printing the token.
12. The committed `.env.example` gains blank `GTPL141_HVAC_BASE_URL` and
    `GTPL141_HVAC_TOKEN` entries; `grep -r` across the frontend repo for the real
    token value and for `GTPL141_HVAC_TOKEN` finds nothing.
13. Frontend: the panel renders on the GTPL-141 detail view and is absent for
    GTPL-114, GTPL-044, and every other machine; the identity test is exact (no
    `includes("141")` substring match).
14. Frontend build passes `check-translations.mjs` with new keys present in en/de/th.
15. With the latest readback `status != "fresh"` (or sample/received age outside
    [-2s, 15s], or machine offline), all controls are disabled with a visible reason.
16. While any command is `queued|claimed|write_completed|uncertain` or any
    `reset_required` is true, every submit/pulse control is disabled.

### A.5 Out of scope

- Gateway endpoints (`claim`/`arm`/`ack`), server-side 100ms pulse timer, a second
  command queue, fake telemetry, DB migrations.
- Any new credential, auth scheme, or heavy CSRF-token machinery.
- Changing upstream operating limits or inventing clamps on control values.
- Multi-machine control; only GTPL-141.

---

## PART B — BACKEND DESIGN (Go / gorilla/mux)

### B.1 Overview and file layout

The adapter is one new handler file plus two small, well-scoped edits to existing
files. It mirrors the conventions already in `handlers/` (package-level `writeJSON`
in `auth.go`, `writeAuthzError` in `authz.go`, `Cache-Control: no-store`, opaque
denials, driver/internal detail stays in the log).

New / changed files:

- `handlers/gtpl141_controls.go` (new) — upstream client, authorization helper,
  seven handlers, request-body validation, audit logging.
- `config/config.go` (edit) — two new config fields.
- `main.go` (edit) — seven route registrations on the `protected` subrouter.
- `.env.example` (edit) — blank entries for the two env vars.

Dependencies: standard library only (`net/http`, `encoding/json`, `net/url`,
`time`, `log`). No new module is added; `go.mod` is unchanged.

### B.2 Final backend route paths (locked)

Chosen under the existing `/api` prefix, GTPL-141-specific and non-arbitrary. The
`protected` subrouter already strips `/api`, so handlers are registered with the
sub-path (consistent with `protected.HandleFunc("/table", ...)`).

| Method | Full path (browser) | Upstream call |
|---|---|---|
| GET  | `/api/controls/gtpl141/latest` | GET `/machines/unit-141/latest` |
| GET  | `/api/controls/gtpl141/history` | GET `/machines/unit-141/history` (only `minutes`, `limit`) |
| GET  | `/api/controls/gtpl141/capabilities` | GET `/controls/unit-141/capabilities` |
| GET  | `/api/controls/gtpl141/commands` | GET `/controls/unit-141/commands` |
| POST | `/api/controls/gtpl141/commands` | POST `/controls/unit-141/commands` |
| POST | `/api/controls/gtpl141/commands/{uuid}/cancel` | POST `/controls/unit-141/commands/{uuid}/cancel` (body `{}`) |
| POST | `/api/controls/gtpl141/commands/{uuid}/review` | POST `/controls/unit-141/commands/{uuid}/review` |

`{uuid}` is constrained in the router to a UUID shape so a malformed value is a
404 at routing time rather than reaching upstream:
`protected.HandleFunc("/controls/gtpl141/commands/{uuid:[0-9a-fA-F-]{8,64}}/cancel", ...)`.
The handler re-validates `{uuid}` with a canonical UUID regex and only the exact
matched value is interpolated into the upstream path (via `url.PathEscape`), so no
other path segment can be injected.

Registration order note: the static `/controls/gtpl141/commands` (GET/POST) is
registered before the `{uuid}` sub-paths; gorilla/mux matches on the distinct
trailing segments, but we register specific-before-generic to match the existing
file's stated convention. All routes include `"OPTIONS"` in `Methods(...)` like
every other protected route, so CORS preflight is answered by the existing layer.

### B.3 Config fields

In `config/config.go`, add to the `Config` struct:

```
// GTPL-141 remote-control adapter (server-only; never sent to the browser).
GTPL141HVACBaseURL string
GTPL141HVACToken   string
```

Populate in `GetConfig()`:

```
GTPL141HVACBaseURL: strings.TrimRight(getEnv("GTPL141_HVAC_BASE_URL", "https://primeosys.com/hvac-dashboard/api"), "/"),
GTPL141HVACToken:   getEnv("GTPL141_HVAC_TOKEN", ""),
```

`TrimRight` on the base URL keeps path joining unambiguous. No startup log line
echoes the token; a single info line notes whether the adapter is configured
(`token present: true/false`) without printing the value. Validation is
deliberately light (unlike `JWT_SECRET`): an empty token is a supported state
(returns 503), not a fatal error, because the operator provisions it out of band.

`.env.example` additions (blank values only):

```
# --- GTPL-141 remote-control adapter (server-only) ----------------------------
# Bearer token for the upstream HVAC control API. Provisioned on the VPS by the
# operator; never commit a real value, never expose to the frontend.
GTPL141_HVAC_TOKEN=
GTPL141_HVAC_BASE_URL=https://primeosys.com/hvac-dashboard/api
```

`.env` (the real, gitignored file) is NOT modified by this work.

### B.4 Authorization helper

Reuse the existing machine-scope machinery. Add one helper in
`gtpl141_controls.go`:

```
const gtpl141Table = "GTPL_141_GT_140E_S7_1200"

// callerMayAccessGTPL141 confirms the authenticated caller is cleared for the
// GTPL-141 physical table, reusing authorizedTables (which already folds in
// fleet access and the per-user grant cache). Writes the opaque error response
// and returns false on any failure, exactly like resolveTable.
func callerMayAccessGTPL141(w http.ResponseWriter, r *http.Request) (*middleware.UserClaims, bool)
```

Behavior, matching `resolveTable`:

- No claims → `writeAuthzError(401, "Authentication required")`.
- `authorizedTables` error → `writeAuthzError(503, "Authorization data is temporarily unavailable. Please try again.")`.
- Table not in the caller's authorized set → `writeAuthzError(403, "You are not authorized to access the requested machine.")`
  (the exact wording already used, so GTPL-141's existence is not revealed).
- Otherwise returns the claims (needed later for the audit line and operator id).

This routes through `authorizedTables → loadMachineGrant`, so fleet accounts
(`hasFleetAccess`) and accounts whose `monitorAccess` normalizes to `GTPL_141`
pass, and the 30s grant cache / revocation behavior is inherited unchanged.

Every one of the seven handlers calls `callerMayAccessGTPL141` first and returns
immediately if it reports false — before any body parse or upstream call.

### B.5 Local CSRF / unsafe-method protection — decision

Investigation result: the repo has **no** CSRF-token mechanism. Its posture is
CORS allowlist + `SameSite=None; Secure; HttpOnly` auth cookie + bearer. Per the
task, we do **not** invent a heavy scheme. Decision:

- For the three POST routes, add a defense-in-depth Origin check: when the request
  carries an `Origin` header, require `middleware.IsOriginAllowed(origin)`; a
  present-but-disallowed origin is rejected with `writeAuthzError(403, "...")`
  before any upstream call. A **missing** `Origin` is allowed through (native apps,
  same-origin navigations, and some proxies strip it; the existing cookie+bearer
  model already gates the request), matching the existing codebase's reliance on
  the cookie for authentication. This mirrors how `EnableCORS` and the WebSocket
  upgrade already treat the allowlist as authoritative for cross-site reads.
- GET routes do not get the Origin gate (they are not state-changing and the
  authz check already applies).

Rationale recorded here so a reviewer sees it was a deliberate, consistent choice,
not an omission.

### B.6 Writable-tag allowlist and body validation

A single source-of-truth table in `gtpl141_controls.go`:

```
type tagKind int
const ( kindBool tagKind = iota; kindInt; kindReal )

type tagSpec struct {
    kind    tagKind
    isPulse bool
}

var gtpl141WritableTags = map[string]tagSpec{
    // Maintained settings (9) — write-once, not pulses.
    "T1_ref_from_T0":        {kindReal, false},
    "Delta_T":               {kindReal, false},
    "Duration_set_aeration": {kindInt,  false},
    "Auto_aeration_enable":  {kindBool, false},
    "Auto_EN":               {kindBool, false},
    "Set_TH_for_heating_mode": {kindReal, false},
    "Continuous_mode":       {kindBool, false},
    "Set_Run_Duration":      {kindInt,  false}, // hours
    "HP_set_from_HMI":       {kindReal, false},
    // Momentary pulses (6) — value true from expected false.
    "Auto_start":                 {kindBool, true},
    "Auto_stop":                  {kindBool, true},
    "Aeration_start_with_WH":     {kindBool, true},
    "Aeration_stop_with_WH":      {kindBool, true},
    "Aeration_start_with_WOH":    {kindBool, true},
    "Aeration_stop_with_WOH":     {kindBool, true},
}

// Explicitly never writable, never shown. Rejected outright even though they are
// simply absent from the allowlist — a named 422 is clearer and documents intent.
var gtpl141HiddenTags = map[string]bool{
    "Manual_EN": true, "Auto_aeration_disable": true,
}
```

**Decoding to reject unknown fields.** Parse the raw body with
`json.Decoder` + `DisallowUnknownFields()` into a typed struct, and keep
`value`/`expected_value` as `json.RawMessage` so the JSON type can be validated
exactly (distinguishing `true` from `1`, integers from floats):

```
type commandRequest struct {
    RequestID     string          `json:"request_id"`
    Tag           string          `json:"tag"`
    Value         json.RawMessage `json:"value"`
    ExpectedValue json.RawMessage `json:"expected_value"`
    Mode          string          `json:"mode"`
    Reason        string          `json:"reason"`
}
```

Any field outside this set → `DisallowUnknownFields` makes `Decode` error → 422.

**Validation order** (all failures → HTTP 422 with a short, non-sensitive message;
no upstream call made):

1. Decode with unknown-field rejection.
2. `tag` non-empty. If `gtpl141HiddenTags[tag]` → 422 "This tag cannot be written."
3. `spec, ok := gtpl141WritableTags[tag]`; if `!ok` → 422 "Unknown or non-writable tag."
4. Validate `value` against `spec.kind`:
   - `kindBool`: must JSON-unmarshal into `bool` (so only `true`/`false`; `1`, `"true"` rejected).
   - `kindInt`: must be a JSON number with no fractional part and within
     `-32768..32767` (S7 INT range). Reject `1.5`, `40000`, strings.
   - `kindReal`: must be a JSON number and finite (reject `NaN`/`Inf`, non-numbers).
     No operating-limit clamping is applied.
5. If `expected_value` present, validate it against the same `spec.kind` with the
   same rules. For pulses the frontend sends `expected_value:false`; the backend
   does not require a specific value, only correct typing.
6. `request_id`: if present, must match a UUID shape (`[0-9a-fA-F-]{8,64}`);
   if absent, the backend does not fabricate one (the upstream API owns command
   identity) — it is forwarded as sent. (Assumption A-3.)
7. `reason`: if blank/absent, set to the derived operator label (§B8). The browser
   can send a `reason`, but the operator identity in it is always the backend's
   derived value, never a browser-supplied principal.
8. `mode`: forwarded as-is if present; the backend does not enumerate modes
   (upstream owns that) but it is a known field so it is not rejected.

The backend never auto-writes a complementary bit: it forwards exactly one tag
write per request. There is no code path that appends a second command.

The validated request is re-serialized server-side (from the typed struct) into
the upstream body, so only known fields reach upstream.

### B.7 Upstream client

```
var gtpl141Client = &http.Client{
    Timeout: 15 * time.Second,
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        return http.ErrUseLastResponse // do not follow redirects on credentialed calls
    },
    // Default Transport keeps TLS verification ON (no custom TLSClientConfig).
}
```

A single internal helper performs every call:

```
func (…)gtpl141Upstream(ctx context.Context, method, upstreamPath string, body []byte) (status int, respBody []byte, err error)
```

- Builds the URL as `cfg.GTPL141HVACBaseURL + upstreamPath` where `upstreamPath`
  is a hardcoded constant per route (plus the validated `{uuid}` escaped with
  `url.PathEscape` for cancel/review). No browser-provided string is ever placed
  in the URL path.
- Creates a fresh `http.NewRequestWithContext`; sets only `Authorization`,
  `Accept`, and (on POST) `Content-Type`. No browser header is copied.
- Request context is derived from `r.Context()` so the server's existing 30s
  request deadline and client disconnects cancel the upstream call too; the 15s
  client timeout is the independent upper bound.
- Reads the upstream body fully (bounded with `io.LimitReader`, e.g. 1 MiB) and
  returns status + bytes.

**Response passthrough + sanitization.** The handler writes:

- `Content-Type: application/json` and `Cache-Control: no-store` (matching the
  codebase).
- The upstream status code verbatim for 200/400/401/403/409/422/429/503, etc.
- The upstream JSON body verbatim **except**: upstream `Set-Cookie` headers are
  never copied; no upstream header other than the status is reflected. The token
  is never in a response because it only ever lived in the request `Authorization`
  header.
- On transport error / timeout → `writeJSON(502/504, {"success":false,"message":"GTPL-141 service is unavailable."})`.
  (502 for connection/transport failure, 504 for context deadline/timeout.)
- On upstream 401 specifically → the body is replaced with a clear integration-auth
  message (`"GTPL-141 integration credentials were rejected upstream."`) and status
  502 is NOT substituted — the browser receives the distinct message with an
  appropriate status (we pass 502 so it is clearly a backend-integration failure,
  not the operator's own session; see Assumption A-4). The operator's Grain session
  is never affected and no other credential is tried.

GET `history` query handling: read `minutes` and `limit` from `r.URL.Query()`,
parse to int, clamp `minutes` to `[5,1440]` and `limit` to `[20,1000]`
(out-of-range or unparseable → clamp to the nearest documented bound; the task
specifies 5..1440 and 20..1000). Build the upstream query string server-side with
only these two keys. No other query parameter is forwarded.

### B.8 Audit log

Reuse the standard `log.Printf` convention (the codebase logs auth and authz
events this way). One line per command POST, written after the upstream call
returns, at both the request and the outcome:

```
log.Printf("gtpl141-audit: operator=%d(%s) ip=%s request_id=%s tag=%s value=%s expected=%s mode=%s upstream_status=%d outcome=%s ts=%s",
    claims.UserID, claims.Username, middleware.ClientIP(r),
    reqID, tag, valueStr, expectedStr, mode, upstreamStatus, outcome,
    time.Now().UTC().Format(time.RFC3339))
```

- Local operator is `claims.UserID` / `claims.Username` from the Grain session.
- The derived `reason` label embeds the local operator:
  `"Grain Technik dashboard command; operator=<claims.UserID>"` (falls back to the
  task's generic label `"Dashboard command; no operator reason requested."` only if
  for some reason no claims are available, which cannot happen past the authz gate).
- `value`/`expected` are logged as their JSON text; these are control values, not
  secrets. The token is never logged.
- The log line documents that the **upstream actor is the shared integration
  identity**, not per-user: a short comment above the log statement states this so
  the shared identity is not misrepresented as per-user in the upstream system.

Cancel and review also emit a one-line audit entry (operator, uuid, action,
upstream status).

### B.9 Backend edge cases

- Token empty → 503 before anything (FR-B1), checked at the top of every handler
  after authz (so an unauthorized caller still gets 403, not 503 — authz first,
  then config, matching "confirm authorized before any upstream call").
  Decision: authz check first, then token-config check; both precede upstream.
- Empty/garbage request body on POST commands → 422 (decode failure).
- `{uuid}` present but not UUID-shaped → 404 at router (regex), re-checked in
  handler as 422 defense-in-depth.
- Upstream returns non-JSON (HTML error page) → pass through its status but wrap
  the body as `{"success":false,"message":"Upstream returned an unexpected response."}`
  to avoid leaking an upstream error page; log the raw first 512 bytes server-side.
- Context cancelled (client navigated away) → upstream call cancels; handler logs
  and returns without writing a partial success.

### B.10 Backend testability

- Unit-testable without a live upstream by making the upstream caller a package
  variable (`var gtpl141Transport httpDoer = gtpl141Client`) where `httpDoer` is a
  one-method interface (`Do(*http.Request) (*http.Response, error)`); tests inject
  a stub. This isolates: token-empty → 503 (asserts zero `Do` calls); authz 403
  (zero `Do` calls); unknown-field/hidden-tag/type validation → 422 (zero `Do`
  calls); history clamping (asserts the outgoing query); header hygiene (asserts no
  `Origin`/`Cookie` on the outgoing request and presence of `Authorization`).
- Authorization is unit-testable by constructing `*middleware.UserClaims` in
  context and seeding the grant cache / using a fleet account, as the existing
  authz paths are exercised.
- Passthrough of upstream status codes is unit-testable with stub responses for
  401/403/409/422/429/503.
- The `.env.example` and no-secret-in-frontend criteria are integration/CI-checkable
  with `grep`.

---

## PART C — FRONTEND DESIGN (Next.js 16 / React 19 / TanStack Query / Radix)

### C.1 Overview

A new client-only controls panel component is rendered inside the existing
per-machine detail surface, and only when the active machine is exactly GTPL-141.
It uses the existing `api()` client (which attaches the cross-site session cookie
via `credentials:"include"`) to call the new backend routes, TanStack Query for
polling, and the existing Radix/shadcn primitives (`Card`, `Button`, `Dialog`/
`AlertDialog`, `Input`, `RadioGroup`, `Checkbox`, `Tooltip`, `Badge`,
`ScrollArea`) with the project's design tokens. The existing `useLanguage().t`
hook provides all user-facing strings.

**Terminology clarification (Assumption A-1).** The task says "same-origin backend
routes" and "same-origin credentialed fetch". In this codebase the dashboard is on
Vercel and the Grain API is at `API_BASE` (`NEXT_PUBLIC_BACKEND_URL ||
"https://www.primeosys.com/backend"`), i.e. cross-site with a `SameSite=None`
cookie. "Same-origin" here means **the Grain backend's own routes via the existing
`api()` client**, not literal same-origin — the panel calls
`api("/api/controls/gtpl141/...")`, never `primeosys.com/hvac-dashboard` directly,
and holds no credential. This matches every other authenticated call in the app.

### C.2 Exact GTPL-141 identity and where the panel mounts

The per-machine detail/menu view is `src/app/menu/[section]/page.tsx`, where the
route param `section` is the machine config key (e.g. `GTPL-141-gT-140E-S7-1200`).
That page already derives `device` from the param.

**Exact identity (no substring match).** GTPL-141 is identified by the canonical
config key:

```
export const GTPL141_MACHINE_KEY = "GTPL-141-gT-140E-S7-1200"; // MACHINE_CONFIG key
export const GTPL141_TABLE = "GTPL_141_GT_140E_S7_1200";        // backend table
```

Gating uses an **exact equality** on the route param against `GTPL141_MACHINE_KEY`,
not `includes`. Additionally, authorization is confirmed via the existing
`useSession().canAccessMachine(GTPL141_MACHINE_KEY)` (which normalizes to
`gtpl_141` and matches the session's machine list exactly — see
`normalizeMachineId`), so a user who routes to the GTPL-141 page but is not
entitled does not see enabled controls. The two existing substring expressions in
`menu/[section]/page.tsx` (`"GTPL-141-"` in `showAnalogMenu`) are left untouched;
the new gating does not reuse them.

Mount point: the controls panel is added as a new menu entry/section for GTPL-141.
Decision: add a new menu item `controls` to the GTPL-141 branch of
`getMenuItems()` guarded by `device === GTPL141_MACHINE_KEY`, routing to a new
page `src/app/menu/controls/[controls]/page.tsx` that renders `<Gtpl141ControlsPanel/>`
only when `param === GTPL141_MACHINE_KEY` (and otherwise renders a "not available"
notice). This keeps all other machines' menus byte-for-byte unchanged (the new
item appears only under the exact-match guard) and follows the existing
`/menu/<path>/<device>` routing pattern used by `auto`, `aerations`, etc.

Rationale for a dedicated route over inlining into every machine page: it is the
narrowest change that cannot leak the panel to another machine, and it matches the
existing one-section-per-route structure.

### C.3 Files to add / change (frontend)

Add:

- `src/lib/gtpl141.ts` — constants (keys above), typed client wrappers around
  `api()` for the seven routes, response TypeScript types, and gating helpers.
- `src/hooks/useGtpl141Controls.ts` — TanStack Query hooks (latest, capabilities,
  commands) with the polling intervals and a 1s freshness ticker, plus mutation
  hooks (submit setting, submit pulse, cancel, review).
- `src/components/controls/Gtpl141ControlsPanel.tsx` — the panel UI (pulse groups,
  settings grid, recent-command list).
- `src/components/controls/` sub-components as needed (`SettingRow`,
  `PulseGroup`, `CommandList`, `UncertainReviewDialog`, `ConfirmSettingDialog`).
- `src/app/menu/controls/[controls]/page.tsx` — the guarded route page.

Change:

- `src/app/menu/[section]/page.tsx` — add the `controls` menu item under the exact
  GTPL-141 guard only.
- `public/locales/en/translation.json`, `de/translation.json`, `th/translation.json`
  — new keys (§C.11).

No change to `apiClient.ts`, `session-provider.tsx`, `language-provider.tsx`,
`query-provider.tsx`, or any other machine's code.

### C.4 Client wrappers (typed) around `api()`

In `src/lib/gtpl141.ts`, thin functions returning parsed JSON, reusing the error
classes already thrown by `api()` (`SessionExpiredError`, `ForbiddenError`,
`RateLimitedError`, `ApiError`):

```
getLatest(signal)        -> api("/api/controls/gtpl141/latest", {signal, cache:"no-store"})
getCapabilities(signal)  -> api("/api/controls/gtpl141/capabilities", {signal})
getCommands(signal)      -> api("/api/controls/gtpl141/commands", {signal})
getHistory({minutes,limit}, signal)
submitCommand(body)      -> api("/api/controls/gtpl141/commands", {method:"POST", body: JSON.stringify(body)})
cancelCommand(uuid)      -> POST .../commands/<uuid>/cancel  body "{}"
reviewCommand(uuid,body) -> POST .../commands/<uuid>/review  body {plc_inspected:true, outcome}
```

All POST bodies are built client-side from only the documented fields. The client
never sends a token or an `Origin` override (the browser sets `Origin` itself; the
backend validates it).

### C.5 Polling / freshness model (TanStack Query)

Three queries, enabled only while the panel is mounted and the browser tab is
visible:

- `["gtpl141","latest"]` — `refetchInterval: 6000`
- `["gtpl141","capabilities"]` — `refetchInterval: 5000`
- `["gtpl141","commands"]` — `refetchInterval: 3000`

No-overlap guarantee: each query uses an `AbortController` via the `signal`
provided by TanStack Query's `queryFn`, and `refetchIntervalInBackground: false`
so a hidden tab stops polling. TanStack Query does not start a new fetch for a key
while its previous fetch is in flight, satisfying "avoid overlapping requests".
Bounded timeout: each `api()` call wraps `fetch` with an `AbortController` that
also aborts after a hard ceiling (e.g. 12s) so a hung request cannot wedge a slot.

Freshness ticker: a `useEffect` sets a 1s `setInterval` storing `now` in state.
All gating (§C.6) is recomputed from `now` each tick, so even if a poll loop
stalls (e.g. tab throttling), displayed data ages out and controls disable within
~1s of crossing a threshold. This is independent of query `staleTime`.

The global `QueryClient` default `staleTime` is 60s; these queries override with
`staleTime: 0` and the intervals above so they always reflect fresh reads.

### C.6 Enable/disable gating (exact rules)

A control (any setting or pulse) is enabled only if ALL hold. Each failed
condition maps to a specific, translated lock message shown in place of an
unexplained disabled state.

Global (from `latest` readback + `capabilities`), recomputed every 1s:

- `latest.status === "fresh"`.
- `latest.sample_ts` and `latest.received_ts` parse to valid timestamps, and each
  age (`now - ts`) is within `[-2s, +15s]` (allow 2s of clock skew; stale beyond
  15s). Unparseable timestamp → not fresh.
- machine online (`latest.online === true`, or the equivalent field the readback
  exposes — Assumption A-5 on the exact field name).
- `capabilities.mode === "live"` (never `simulated`).
- `capabilities.gateway.ready === true`.
- `capabilities.gateway.heartbeat_age` within `[-2s, +12s]`.
- No command is in a blocking state: none of the `commands` list is
  `queued | claimed | write_completed | uncertain`, and no entry has
  `reset_required === true`. (One action at a time.)

Per-tag (additionally):

- The tag is present in BOTH `capabilities.live_tags` and
  `capabilities.gateway.tags`.
- For a maintained setting: its readback value is not null. If null, the control
  is disabled with the generic "Awaiting updated gateway readback" — and for the
  duration tag (`Duration_set_aeration`) that exact message is shown as specified.
- For a pulse (additionally to the above): `capabilities.pulse_enabled === true`,
  the capability's `pulse_ms` matches the expected/advertised value, and the
  selected bit in `capabilities.gateway.pulse_values[tag]` is **exactly** `false`.
  A missing or `null` entry is NOT treated as false → disabled.

When a control is disabled, the UI renders a tooltip + inline helper text with the
first failing reason (translated), never a bare greyed-out button.

### C.7 Submitting a maintained setting

- On "edit", capture the current readback value as `expected_value` and the
  operator's requested value; display both ("selected/actual" vs "requested").
- Generate a stable UUID once per operator action with `crypto.randomUUID()`,
  stored in component state (not persisted to `localStorage`/`sessionStorage`),
  and reused on retries so the upstream command is idempotent per action.
- A deliberate confirm is required (`AlertDialog` with the actual-vs-requested
  summary and a confirm button). No typed reason is required or offered as
  mandatory.
- Conflict handling: if, between capture and confirm, the readback's actual value
  changes (HMI moved it), surface a conflict state ("Value changed on the HMI —
  review before sending") and require re-review; never silently resubmit, never
  optimistically overwrite the shown actual with the requested value. Pending
  requests are shown as a separate "pending" badge; the displayed actual only ever
  updates from a readback, never from the submitted value.
- Submit body: `{request_id, tag, value, expected_value}` (plus `mode` only if the
  UI exposes one; `reason` omitted so the backend supplies the operator label).

### C.8 Pulses

- A pulse group (Auto, Aeration-with-heating, Aeration-without-heating) renders two
  icon buttons (start/stop) mapped to the six pulse tags.
- Pressing a pulse sends ONE command: `{request_id, tag, value:true, expected_value:false}`.
- No `value:false` follow-up command, no client-side 100ms timer. The 100ms pulse
  behavior is entirely upstream/gateway responsibility.

### C.9 Command status handling

Status labels (translated) and behavior for each state the backend/upstream
reports:

| Status | UI treatment |
|---|---|
| `queued` | "Queued"; cancel allowed; blocks new actions |
| `claimed` | "Claimed by gateway"; no cancel; blocks new actions |
| `write_completed` | "Write completed, verifying"; blocks new actions |
| `readback_verified` | "Verified" (terminal success) |
| `pulse_completed` | "Pulse completed" (terminal success) |
| `rejected` | "Rejected" (terminal, show detail) |
| `expired` | "Expired" (terminal) |
| `cancelled` | "Cancelled" (terminal) |
| `uncertain` | "Needs review" → opens the review flow; blocks new actions |
| `reviewed` | "Reviewed" (terminal) |
| `simulated` | "Simulated (not a live change)" — explicitly NOT shown as success |

- Cancel: enabled only for `queued`. `POST .../commands/{uuid}/cancel` with body
  `{}`. A 409 response means the command already left the queue — show the current
  state detail and refresh the `commands` query; do not retry.
- Uncertain review: a `RadioGroup` with exactly three outcomes
  (`requested_change_confirmed`, `different_state_confirmed`, `unable_to_confirm`),
  nothing preselected. A separate "I inspected the PLC/HMI directly" `Checkbox`,
  not preselected; changing the radio selection clears the checkbox. `POST review`
  is enabled only for the two confirmable outcomes
  (`requested_change_confirmed`, `different_state_confirmed`) AND only when the
  inspection checkbox is checked, sending
  `{"plc_inspected":true,"outcome":"<selected>"}`. `unable_to_confirm` does not POST
  a review (it is an acknowledgement that no determination was made).

Recent-command list: a compact `ScrollArea` of the latest commands from the
`commands` query (uuid short form, tag, status badge, timestamp), newest first.

### C.10 Error handling by HTTP status (frontend)

Mapped from the typed errors `api()` throws and from `ApiError.status`:

- 401 (`SessionExpiredError`): the client already routes to login; the panel shows
  an integration/session message and never offers a password fallback.
- 403 (`ForbiddenError`): "You are not authorized to control this machine."; panel
  stays read-only.
- 409 (`ApiError` 409): state conflict — show the returned detail and refetch the
  relevant query (`commands`/`latest`).
- 422 (`ApiError` 422): validation — surface the backend message near the control.
- 429 (`RateLimitedError`): back off using `retryAfter`; disable submit until then.
- 503 / network / 502 / 504: "GTPL-141 service unavailable" and **disable all
  write controls** while unresolved (treated like a failed freshness check).
- All `api()` calls have a bounded client timeout (§C.5).

### C.11 i18n keys to add (en, de, th)

`check-translations.mjs` scans `t("literal")` calls in `src/**` and fails the build
if any literal key is missing/blank in en/de/th, or (for de/th) if the value equals
the key (unless in `SAME_IN_EVERY_LOCALE`). Therefore every new string must use a
`t("…")` literal and be added with a genuine translation to all three catalogues.

Planned keys (final English copy may be refined in implementation; all three
locales must receive real translations, not key echoes):

- Panel: `"Remote Control"`, `"GTPL-141 Remote Control"`, `"Controls"`.
- Pulse groups: `"Auto"`, `"Aeration (with heating)"`, `"Aeration (without heating)"`,
  `"Start"`, `"Stop"`. (`"Start"`, `"Auto"` already exist / are in
  `SAME_IN_EVERY_LOCALE`; reuse rather than duplicate.)
- Settings: a label per maintained setting
  (`"T1 reference from T0"`, `"Delta T"`, `"Aeration duration"`,
  `"Auto aeration enable"`, `"Auto enable"`, `"Set TH for heating mode"`,
  `"Continuous mode"`, `"Set run duration (hours)"`, `"HP set from HMI"`).
- Actions/dialogs: `"Confirm change"`, `"Requested"`, `"Current"`, `"Send command"`,
  `"Cancel"`, `"Pending"`, `"Value changed on the HMI — review before sending"`.
- Gating/lock messages: `"Awaiting updated gateway readback"`,
  `"Readback is stale"`, `"Machine is offline"`, `"Gateway not ready"`,
  `"Control mode is not live"`, `"This control is not available right now"`,
  `"A command is already in progress"`, `"Reset required on the gateway"`.
- Command statuses: `"Queued"`, `"Claimed by gateway"`, `"Write completed, verifying"`,
  `"Verified"`, `"Pulse completed"`, `"Rejected"`, `"Expired"`, `"Cancelled"`,
  `"Needs review"`, `"Reviewed"`, `"Simulated (not a live change)"`.
- Review flow: `"Needs review"`, `"Requested change confirmed"`,
  `"Different state confirmed"`, `"Unable to confirm"`,
  `"I inspected the PLC/HMI directly"`, `"Submit review"`.
- Errors: `"You are not authorized to control this machine."`,
  `"GTPL-141 service unavailable"`, `"Too many attempts, please wait"`,
  `"GTPL-141 remote control is not configured."`,
  `"GTPL-141 integration credentials were rejected."`.

Implementation note: before build, run `node scripts/check-translations.mjs` and
add any key it flags. Keys identical across locales that are genuine (abbreviations
like tag names) can be added to `SAME_IN_EVERY_LOCALE` only if truly identical; the
preference is a real translation.

### C.12 Lifecycle

The panel's queries live only while the component is mounted. On unmount
(navigation away, tab close), TanStack Query stops the intervals and the
`AbortController`s abort in-flight reads. No unmount/cleanup path calls
`submitCommand`, `cancelCommand`, mode changes, or any write — closing the browser
only stops UI polling, exactly as required.

### C.13 Frontend testability

- Unit (component) testable with mocked `api()` responses: gating truth-table
  (fresh/stale/offline, mode live/simulated, heartbeat in/out of range, pulse
  false/null, command-in-progress), status-label rendering, uncertain-review
  checkbox-clearing behavior, cancel-only-when-queued.
- The exact-identity guard is unit-testable: render under `GTPL-141-gT-140E-S7-1200`
  (panel shows) vs `GTPL-114-GT-140E-S7-1200`, `GTPL-044-GT-140E-S7-1200`, and a
  bare `"141"`-containing string (panel hidden).
- Freshness ticker is testable with fake timers (advance 1s, assert disable).
- i18n completeness is enforced by `check-translations.mjs` in CI/build.
- Playwright (`tests/`, `playwright.config.ts` already present) can cover the happy
  path against a stubbed backend, but is not required for the unit-level criteria.

---

## PART D — ASSUMPTIONS (explicitly unverified)

- **A-1 "Same-origin" wording.** Interpreted as "the Grain backend's own routes via
  the existing credentialed `api()` client", because the real deployment is
  cross-site (Vercel → `primeosys.com/backend`) with a `SameSite=None` cookie. The
  panel never calls the upstream HVAC host directly. If literal same-origin
  (Next.js route handlers proxying to the backend) is intended, the frontend would
  add `src/app/api/controls/gtpl141/*` route handlers instead; this is NOT the
  chosen approach and would duplicate credential handling the backend already owns.
- **A-2 Upstream response shapes.** The exact JSON field names for `latest`
  (`status`, `sample_ts`, `received_ts`, `online`) and `capabilities`
  (`mode`, `gateway.ready`, `gateway.heartbeat_age`, `live_tags`, `gateway.tags`,
  `pulse_enabled`, `pulse_ms`, `gateway.pulse_values`) are taken from the task's
  gating description. The backend passes bodies through verbatim, so if upstream
  uses different names the frontend gating field map (centralized in
  `src/lib/gtpl141.ts`) is the single place to adjust. The backend does not depend
  on these shapes.
- **A-3 `request_id` ownership.** The backend forwards a client-supplied,
  UUID-shaped `request_id` and does not fabricate one, on the assumption the
  upstream command API treats `request_id` as the idempotency key. If upstream
  requires the backend to mint it, that is a one-line change in the command handler.
- **A-4 Upstream 401 surfaced status.** Chosen to return 502 with a distinct
  integration-auth message (so it is never confused with the operator's own Grain
  401, which would trigger a logout redirect). If the reviewer prefers passing the
  raw 401 through with a flag the frontend can distinguish, that is an alternative;
  the constraint satisfied either way is "no credential fallback, operator session
  untouched".
- **A-5 `latest.online` field.** The exact "machine online" field in the readback
  is assumed; it is centralized in the frontend gating helper and does not affect
  the backend.
- **A-6 Menu mount.** The controls entry is added as a new `/menu/controls/<device>`
  route guarded by exact identity. If the intended host surface is a different
  machine-detail view, the panel component is self-contained and can be mounted
  elsewhere behind the same exact-identity guard without other changes.
- **A-7 `GTPL141_HVAC_TOKEN` empty = 503, not fatal.** Unlike `JWT_SECRET`, an empty
  control token is a supported runtime state, so the process still starts.

---

## PART E — DESIGN REVIEW RESPONSES

No `design-review.json` / `design-review.md` was present at authoring time, so this
section is empty. On a future revision, each review finding will be listed here as
addressed / backlogged (with reason) / justified-ignored, consistent with the
requirements above.

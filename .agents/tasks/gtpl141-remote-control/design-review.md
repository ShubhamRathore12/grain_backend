# Design Review — GTPL-141 Remote-Control Integration

Reviewer: design-review subagent (fresh read, no authoring context).
Design under review: `.agents/tasks/gtpl141-remote-control/design.md`.
Scope: Go backend adapter (`grain_backend`) + Next.js controls panel (`new_plc_software`).

Verdict: **CHANGES_REQUESTED** (1 HIGH, 3 MEDIUM, several NITs).

The design is strong on the security-critical axes — the secret never leaves the
backend, the token lives only in server env with a blank-token 503 state, the
authorization helper genuinely reuses `authorizedTables`/`loadMachineGrant`, the
writable-tag allowlist and hidden-tag rejection are concrete, and the explicit
NOTs (no second queue, no pulse timer, no complementary-bit auto-write, no DB
migration) are honored. The blocking issues are integration-contract mismatches
with the actual frontend `api()` client, one internal contradiction in the
`.env.example` content, and a wrong i18n assumption that will break the build if
taken at face value.

---

## Findings

### 1. HIGH — Frontend `api()` returns a raw `Response`, not parsed JSON; the §C.4 wrappers and §C.10 error contract are written against a client that does not behave that way.

Where: §C.4 "Client wrappers (typed) around `api()`" and §C.10 "Error handling by HTTP status".

Verified against `new_plc_software/src/lib/apiClient.ts`:

- `api(path, init)` resolves to `Promise<Response>` (the raw fetch `Response`). It
  throws typed errors **only** for 401 (`SessionExpiredError`), 403
  (`ForbiddenError`), and 429 (`RateLimitedError`). For every other status —
  including 409, 422, 502, 503, 504 — it returns the `Response` **without
  throwing and without parsing**.
- The helper that parses JSON and throws `ApiError` (with `.status`) for other
  non-2xx is `apiJson<T>()`, a separate export.

The design's §C.4 writes `getLatest(signal) -> api("/api/controls/gtpl141/latest", …)`
and describes these as "thin functions returning parsed JSON, reusing the error
classes already thrown by `api()`". But `api()` returns an unparsed `Response` and
does **not** throw `ApiError` for 409/422. §C.10 then says "409 (`ApiError` 409)"
and "422 (`ApiError` 422)" arrive as typed `ApiError` with `.status` — which is
only true through `apiJson()`, never through `api()`.

Impact: as written, every read wrapper would hand components an unparsed
`Response`, and the 409/422/503/502/504 handling in §C.10 (the HMI-conflict flow,
the validation-message surfacing, the service-unavailable disable-all) would never
fire because no `ApiError` is thrown. This breaks FR-F5 (surface HMI conflict),
FR-F7/F8 (status-specific handling), and the §C.10 contract.

Concrete fix: build the wrappers on `apiJson<T>()` (which parses and throws
`ApiError` for non-2xx), not `api()`. Example:

```ts
import { apiJson, ApiError } from "@/lib/apiClient";

export async function getLatest(signal?: AbortSignal): Promise<Gtpl141Latest> {
  return apiJson<Gtpl141Latest>("/api/controls/gtpl141/latest", {
    method: "GET", cache: "no-store", signal,
  });
}

export async function submitCommand(body: Gtpl141CommandBody): Promise<Gtpl141CommandResult> {
  return apiJson<Gtpl141CommandResult>("/api/controls/gtpl141/commands", {
    method: "POST", body: JSON.stringify(body),
  });
}
```

and keep §C.10's status mapping keyed off `err instanceof ApiError && err.status`
(for 409/422/503/502/504) plus the dedicated `SessionExpiredError` /
`ForbiddenError` / `RateLimitedError` classes for 401/403/429. State this
explicitly in the design so the implementer does not wire `api()` directly.

Note the second-order effect this fixes: `api()` auto-fires the global
session-expiry teardown on 401. For the integration-auth case the backend returns
**502** on upstream 401 (design §B.7 / Assumption A-4), so it will not trigger the
operator logout — good — but that only holds because the backend does not pass the
raw 401 through. Keep A-4 as decided (502), and the §C.10 "401 never offers a
password fallback" wording should be reconciled with the fact that a true operator
401 is handled globally by `api()`/`apiJson()` already.

### 2. MEDIUM — `.env.example` snippet (§B.3) sets a non-blank `GTPL141_HVAC_BASE_URL`, contradicting the design's own acceptance criterion A.4-12 ("blank entries") and NFR-2 ("only blank/example entries committed").

Where: §B.3 "`.env.example` additions" vs §A.4 criterion 12 and §A.3 NFR-2.

The §B.3 block commits:

```
GTPL141_HVAC_TOKEN=
GTPL141_HVAC_BASE_URL=https://primeosys.com/hvac-dashboard/api
```

Criterion 12 states the committed `.env.example` "gains **blank**
`GTPL141_HVAC_BASE_URL` and `GTPL141_HVAC_TOKEN` entries". The token is blank
(the security-critical part is satisfied), but the base URL is not blank — it is
pre-filled with the production host. This is an internal contradiction the
implementer must resolve, and the base URL being committed also pins a production
endpoint into the template.

Note this is not a secret leak (the base URL is not a credential, and the
`getEnv` default in `GetConfig()` already supplies the same URL at runtime), so it
is MEDIUM, not HIGH. But it is a direct conflict between two sections of the
design.

Concrete fix: make the committed template entry blank and rely on the code
default, consistent with criterion 12:

```
# Optional override; defaults to the production HVAC API when blank.
GTPL141_HVAC_BASE_URL=
GTPL141_HVAC_TOKEN=
```

If a pre-filled URL in the template is actually desired, change criterion 12 to
say "blank token, example base URL" so the two sections agree — pick one.

### 3. MEDIUM — i18n: the design asserts `"Auto"` already exists / is in `SAME_IN_EVERY_LOCALE`; it is in neither, so taking that at face value will fail `check-translations.mjs`.

Where: §C.11, bullet "Pulse groups": `("Start", "Auto" already exist / are in SAME_IN_EVERY_LOCALE; reuse rather than duplicate.)`.

Verified against `new_plc_software/scripts/check-translations.mjs` and the three
catalogues:

- `SAME_IN_EVERY_LOCALE` contains `Start`, `Live`, `live`, product/instrument
  tokens — but **not** `Auto`.
- `public/locales/{en,de,th}/translation.json` contain `Start` and `Stop` but
  **no** `Auto` key.

The script's `missing` check applies to **all** locales including `en`, so a
`t("Auto")` literal with no `"Auto"` key anywhere fails the build immediately; and
for de/th a value equal to the key fails the untranslated check unless the key is
added to `SAME_IN_EVERY_LOCALE`. Because the design tells the implementer `"Auto"`
already exists, it risks being omitted — breaking FR-F10 / NFR-4 (`npm run build`
green).

Concrete fix: treat `"Auto"` as a new key. Add it to all three catalogues with a
genuine value (e.g. de `"Auto"`, th appropriate translation), and if the de value
legitimately equals the key, add `"Auto"` to `SAME_IN_EVERY_LOCALE`. Correct the
§C.11 note to list only `Start` (and `Stop`) as pre-existing; `Live`/`live` are
the only control-relevant tokens already allowlisted.

### 4. MEDIUM — Reused-but-unverified gating field names are presented in the body (§C.6) as if specified, while the design elsewhere (A-2/A-5) admits they are guesses; the two readings conflict and the frontend DOES depend on them.

Where: §C.6 (hard rules referencing `latest.status`, `latest.sample_ts`,
`latest.received_ts`, `latest.online`, `capabilities.mode`,
`capabilities.gateway.ready`, `capabilities.gateway.heartbeat_age`,
`capabilities.live_tags`, `capabilities.gateway.tags`,
`capabilities.pulse_enabled`, `capabilities.gateway.pulse_values`) vs Assumptions
A-2 and A-5.

§C.6 states these as exact, testable gating conditions ("A control is enabled only
if ALL hold"), but A-2/A-5 concede the field names are "taken from the task's
gating description" and unverified against a live upstream. The backend passes
bodies through verbatim and does not depend on them (verified: §B.7 passthrough is
byte-level), so the risk is isolated to the frontend — but it is real: if any
field name is wrong, every control silently stays disabled (fail-closed), which
looks like a broken panel rather than a config issue.

This is not downgradable to a NIT because the whole enable/disable behavior
(FR-F3/F4, criteria 15/16) rides on these names and they are unverifiable from the
repos alone (upstream API is external).

Concrete fix: keep the single-source-of-truth field map in `src/lib/gtpl141.ts`
(as A-2 already plans) AND add to the design one concrete captured sample of each
upstream response (`/machines/unit-141/latest`, `/controls/unit-141/capabilities`)
— even a documented example from the upstream API docs — so the field names are
pinned before implementation rather than guessed. If no sample is obtainable
pre-implementation, state the fail-closed behavior explicitly as intended
(disabled-with-reason when an expected field is absent) and add a visible
"gating fields unavailable" diagnostic so a wrong field name is distinguishable
from a genuinely stale/offline machine.

### 5. NIT — §C.13 test vectors reference machine keys that are not `MACHINE_CONFIG` entries in the form given.

Where: §C.13 "exact-identity guard is unit-testable … vs `GTPL-114-GT-140E-S7-1200`".

Verified against `src/lib/machineConfig.ts`: there is no `GTPL-114-…` key — the
GTPL_114 **table** is reached via the `"GTPL-030-gT-180E-S7-1200"` config key.
`"GTPL-044-GT-140E-S7-1200"` does exist exactly. The exact-match test is sound; the
negative example just names a non-existent key.

Concrete fix: use real negative keys that exist in `MACHINE_CONFIG`, e.g.
`"GTPL-030-gT-180E-S7-1200"` (the GTPL_114-table machine), `"GTPL-044-GT-140E-S7-1200"`,
and a bare `"141"` string, so the test asserts the panel is hidden for a real
sibling machine and for a substring that must not match.

### 6. NIT — §C.11 lists `"Needs review"` twice (status label and review-flow heading); confirm it is intentionally one shared key.

Where: §C.11 "Command statuses" and "Review flow" both list `"Needs review"`.

Not a bug — one key reused is fine and the translation check is satisfied by a
single catalogue entry — but the design should say so explicitly so the
implementer does not create two divergent keys.

Concrete fix: add a one-line note that `"Needs review"` is a single shared key.

### 7. NIT — Upstream-timeout status mapping is slightly ambiguous given the 15s client timeout sits inside the 30s request deadline.

Where: §B.7 "502 for connection/transport failure, 504 for context
deadline/timeout."

Verified: `main.go` `requestDeadlineMiddleware` sets a 30s request context (4m for
`/api/export`), and the design's upstream client timeout is 15s. The 15s client
timeout therefore fires first for a slow upstream, surfacing as a client timeout
error rather than a context-deadline cancellation. The design should map the
15s-client-timeout case to 504 as well (it is a timeout, not a transport failure),
so the taxonomy is unambiguous.

Concrete fix: state "transport/connection error → 502; any timeout (client 15s
timeout OR context deadline) → 504", and detect via
`errors.Is(err, context.DeadlineExceeded)` plus the `http.Client` timeout error
(`os.IsTimeout` / `net.Error.Timeout()`).

---

## Verified Assumptions

- `writeJSON` (package-level, sets `Content-Type: application/json` +
  `Cache-Control: no-store`) exists in `handlers/auth.go` — §B.1/§B.7 correct.
- `writeAuthzError(w, status, message)` with opaque wording exists in
  `handlers/authz.go`; the 403 string `"You are not authorized to access the
  requested machine."` is the real, existing wording — §B.4 correct.
- `authorizedTables` → `loadMachineGrant` folds in fleet access
  (`hasFleetAccess`) and the 30s grant cache (`grantCacheTTL`); fleet accounts and
  accounts whose `monitorAccess` normalizes to the GTPL-141 key pass — §B.4
  correct. When `EnforceMachineScope` is false, all tables are returned (break-
  glass), so GTPL-141 is in-set and any authed caller passes; acceptable and
  consistent.
- Physical table `GTPL_141_GT_140E_S7_1200` exists in `handlers/machine.go`
  (`machineStatusTables`) and normalizes to `GTPL_141` via `normalizeMachineKey`
  — the authz allowlist genuinely covers it.
- `middleware.IsOriginAllowed(origin)` exists (`middleware/cors.go`), returns
  false for empty origin, and is the same allowlist used by CORS and the
  WebSocket upgrade — §B.5 Origin-gate decision is consistent with existing
  posture; a missing `Origin` passing through is consistent with the cookie-first
  model. CORS posture is allowlist + credentialed, no CSRF-token mechanism exists
  — §B.5 investigation claim correct.
- `middleware.UserClaims{Username, AccountType, UserID int}` and
  `middleware.GetUserFromContext` exist (`middleware/auth.go`) — the audit-log and
  operator-label fields (§B.8) are real.
- `middleware.ClientIP(r)` exists (`middleware/security.go`) — §B.8 correct.
- `config.getEnv(key, default)` exists and `strings` is already imported in
  `config/config.go` — §B.3 `getEnv`/`strings.TrimRight` compiles.
- `protected := r.PathPrefix("/api").Subrouter()` with `AuthenticateToken` and
  sub-path registration (`protected.HandleFunc("/table", …)`) — §B.2 route
  registration pattern (sub-path without `/api`) is correct, as is the
  specific-before-generic ordering note and `OPTIONS`-in-`Methods` convention.
- Server has a 30s request-deadline middleware (`requestDeadlineMiddleware`) and
  5m write timeout — §B.7 "existing 30s request deadline bounds the upstream call"
  is correct; the independent 15s client timeout sits inside it.
- Backend 401 handling: upstream 401 → backend returns 502 with distinct message,
  so the operator's own `api()` 401 teardown is not triggered — consistent with
  Assumption A-4 and the frontend client's 401 behavior.
- Frontend `MACHINE_CONFIG` key for GTPL-141 is exactly
  `"GTPL-141-gT-140E-S7-1200"` (lowercase `gT`) — §C.2 `GTPL141_MACHINE_KEY`
  matches; backend table constant `GTPL_141_GT_140E_S7_1200` matches `GTPL141_TABLE`.
- `useSession().canAccessMachine` → `sessionHasMachine` → `normalizeMachineId`
  (regex `gtpl[-_ ]?(\d{2,4})` → `gtpl_141`) exists — §C.2 entitlement check is
  real. Note it is a normalized (not literal-exact) match, so exactness is
  enforced by the route-param equality, not by `canAccessMachine`; the design's
  dual-gate description is accurate.
- Menu routing: `menu/[section]/page.tsx` uses the machine key as the route param
  and links menu items to `/menu/${item.path}/${device}`; a `controls` item would
  route to `/menu/controls/<machineKey>`, so the proposed
  `src/app/menu/controls/[controls]/page.tsx` route (§C.2) is structurally
  consistent with existing sections (`auto`, `inputs/analog`, …).
- `check-translations.mjs` scans `t("literal")` across `src/**`, enforces presence
  in en/de/th, and flags de/th values equal to the key unless allowlisted — §C.11
  description is accurate. `package.json` `build` = `node
  scripts/check-translations.mjs && next build` — NFR-4 correct.
- `apiClient.ts` exports `SessionExpiredError`, `ForbiddenError`,
  `RateLimitedError`, `ApiError` with `.status`/`.body`, and `RateLimitedError`
  carries `retryAfter` from the `Retry-After` header — §C.10 class names are real.
- `tests/` (Playwright e2e) and `playwright.config.ts` exist — §C.13 is accurate.
- No pre-existing `GTPL141_HVAC_*` or upstream-token references exist in either
  repo's source (only in the design doc) — nothing to clean up, no prior leak.

## Unverified / Wrong Assumptions

- **WRONG — `"Auto"` i18n key** (Finding 3): `"Auto"` is neither a catalogue key
  nor in `SAME_IN_EVERY_LOCALE`. The design's "already exists / reuse" note is
  incorrect; it must be added as a new key.
- **WRONG/CONFLICTING — `.env.example` base URL** (Finding 2): §B.3 commits a
  non-blank base URL, contradicting the design's own criterion A.4-12 and NFR-2.
- **WRONG test vector — `GTPL-114-GT-140E-S7-1200`** (Finding 5): not a
  `MACHINE_CONFIG` key (GTPL_114 table is reached via the `GTPL-030-…` key).
- **UNVERIFIED (external) — upstream response field names** (A-2, A-5, Finding 4):
  `latest.{status,sample_ts,received_ts,online}` and `capabilities.*` /
  `gateway.*` shapes are taken from the task description, not confirmed against the
  live upstream HVAC API (external, not in either repo). The backend passthrough
  does not depend on them; the frontend gating fully does.
- **UNVERIFIED (external) — `request_id` idempotency ownership** (A-3): whether
  upstream treats `request_id` as the idempotency key, and whether it mints a
  command id when none is supplied, is assumed. Cannot be confirmed from the repos.
- **UNVERIFIED (external) — upstream route paths and `unit-141` naming** (§B.2):
  `/machines/unit-141/latest`, `/controls/unit-141/{capabilities,commands,...}`
  are hardcoded per the task but not verifiable against the external upstream.
  Correctly hardcoded server-side (non-arbitrary, no browser input) — this is the
  right posture regardless.
- **UNVERIFIED (external) — upstream 401 body/semantics** (A-4): the choice to
  surface upstream 401 as backend 502 with a distinct message is sound and
  internally consistent; the exact upstream 401 body is external.

---

## Explicit-NOTs check (all honored by the design)

- No gateway `claim`/`arm`/`ack` proxy — only the seven read/command/cancel/review
  routes in §B.2; confirmed out-of-scope in §A.5.
- No server-side or client-side 100ms pulse timer — §B.6 forwards exactly one tag
  write; §C.8 sends one `{value:true, expected_value:false}` with no follow-up and
  no timer.
- No second queue — §A.5 and §B.6 (no appended command path).
- No fake telemetry — passthrough only; `simulated` explicitly never shown as live
  success (§C.9, criterion aligned).
- No DB migrations — §A.5; backend touches only `config.go`, `main.go`,
  `.env.example`, and the new handler file (standard library only, `go.mod`
  unchanged).
- No complementary-bit auto-write — stated in §B.6 and criterion 10.
- Token never frontend-exposed, never `VITE_`/`NEXT_PUBLIC_`-prefixed, never
  hardcoded — honored: token read only via server `getEnv`, frontend calls only
  `/api/controls/gtpl141/*` through credentialed `api()`, no `primeosys/hvac`
  reference exists in frontend source.

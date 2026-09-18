package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"grain_backend/config"
	"grain_backend/database"
	"grain_backend/middleware"
)

// -----------------------------------------------------------------------------
// Machine-scope authorization (S-02, S-06, S-07)
//
// The old contract let any caller name a database table in a query parameter and
// receive its contents. Two things were missing: a session, and a check that the
// session owns the machine behind that table.
//
// This file supplies the second half. Every handler that reads a `table`
// parameter now routes through resolveTable, which:
//
//	1. requires an authenticated caller,
//	2. maps the parameter to a known physical table (server-side allowlist),
//	3. confirms the caller's monitorAccess grant covers that machine,
//	4. answers unknown and unauthorized identically, so the API cannot be used
//	   to discover which machines exist.
// -----------------------------------------------------------------------------

// legacyDefaultTable is the demo table the API used to fall back to for any
// request that omitted `table`. It is kept only as a last resort: it is empty and
// lacks a FAULT_CODE column, so defaulting to it made /api/faultLogs answer 500
// for accounts whose real machines were fine. Prefer a machine the caller
// actually has.
const legacyDefaultTable = "kabomachinedatasmart200"

// defaultTableFor picks the machine to use when the caller did not name one.
// permitted is already sorted, so the choice is stable across requests.
func defaultTableFor(permitted []string) string {
	for _, table := range permitted {
		if table != legacyDefaultTable {
			return table
		}
	}
	return permitted[0]
}

// wildcardGrants are monitorAccess values that mean "every machine". Grain
// Technik staff accounts use these; customer accounts list machines explicitly.
var wildcardGrants = map[string]bool{
	"*": true, "ALL": true, "ALL_MACHINES": true, "EVERY": true, "FULL": true,
}

// placeholderGrants are values that appear in the monitorAccess column but do not
// name a machine, so they must be skipped rather than treated as an unknown
// identifier.
//
// The live data contains two kinds of these: "0" as a "nothing set here"
// placeholder (the two staff accounts), and "devices" — a UI translation key that
// leaked into user data on 15 of 17 accounts. Treating either as a machine name
// produced a hard 403 for a request that was really just a client-side bug.
var placeholderGrants = map[string]bool{
	"0": true, "-": true, "": true, "NONE": true, "NULL": true, "NIL": true,
	"N/A": true, "NA": true, "UNDEFINED": true,
	"DEVICES": true, "DEVICE": true, "DEVICE_ACTIVITY": true,
}

// isPlaceholderIdentifier reports whether a raw grant or request value is a
// known non-machine token.
func isPlaceholderIdentifier(raw string) bool {
	return placeholderGrants[strings.ToUpper(strings.TrimSpace(raw))]
}

// normalizeMachineKey reduces any spelling of a machine identifier to a single
// canonical key, so all of these land on "GTPL_115":
//
//	GTPL-115-gT-180E-S7-1200   (frontend route segment)
//	GTPL_115_GT_180E_S7_1200   (physical table name)
//	gtpl_115                   (short form)
//	GTPL 115                   (hand-entered grant)
//
// Numbers are zero-padded to three digits so GTPL_81 and GTPL_081 match.
func normalizeMachineKey(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	if wildcardGrants[s] {
		return "*"
	}
	// Not a machine name at all — caller treats "" as "nothing to match".
	if placeholderGrants[s] {
		return ""
	}

	replacer := strings.NewReplacer("-", "_", " ", "_", ".", "_", "/", "_")
	s = replacer.Replace(s)

	// The legacy demo table has no GTPL number.
	if strings.HasPrefix(s, "KABOMACHINEDATASMART") || s == "KABO_200" || s == "KABO200" {
		return "KABO_200"
	}

	idx := strings.Index(s, "GTPL")
	if idx < 0 {
		return s
	}
	rest := strings.TrimLeft(s[idx+len("GTPL"):], "_")
	digits := strings.Builder{}
	for _, c := range rest {
		if c < '0' || c > '9' {
			break
		}
		digits.WriteRune(c)
	}
	number := digits.String()
	if number == "" {
		return s
	}
	for len(number) < 3 {
		number = "0" + number
	}
	return "GTPL_" + number
}

// tableIndex maps every canonical machine key to its physical table. Built once
// from machineStatusTables so the authorization allowlist and the status
// dashboard can never disagree about which machines exist.
var (
	tableIndex     map[string]string
	tableIndexOnce sync.Once
)

func machineTableIndex() map[string]string {
	tableIndexOnce.Do(func() {
		tableIndex = make(map[string]string, len(machineStatusTables))
		for table, displayName := range machineStatusTables {
			// Index by both the display name and the table name: a grant may be
			// recorded either way.
			for _, candidate := range []string{displayName, table} {
				key := normalizeMachineKey(candidate)
				if key == "" || key == "*" {
					continue
				}
				if existing, clash := tableIndex[key]; clash && existing != table {
					log.Printf("⚠️  machine key %q maps to both %q and %q; keeping %q", key, existing, table, existing)
					continue
				}
				tableIndex[key] = table
			}
		}
	})
	return tableIndex
}

// -----------------------------------------------------------------------------
// Per-user grants
// -----------------------------------------------------------------------------

type machineGrant struct {
	tables    map[string]bool // physical table names the user may read
	allAccess bool
	loadedAt  time.Time
}

// -----------------------------------------------------------------------------
// Fleet-wide access
//
// Two different permissions were previously answered by one hardcoded username
// list: "may manage users" and "may see every machine". They are separated here,
// because the second one is what decides cross-customer data exposure and so it
// needs to be reviewable and configurable without a rebuild.
//
// Grain Technik staff need the whole fleet. Customers must stay scoped to their
// own machines by monitorAccess. Defaults are deliberately narrow: the two known
// staff accounts and the accountType only they carry ("manufactura"). The eight
// "manufacturer" accounts are NOT included — granting them the fleet by guess
// would reopen exactly the cross-customer exposure S-02 reported. Add them
// explicitly via env once their role is confirmed.
// -----------------------------------------------------------------------------

const (
	defaultFleetUsernames    = "Narayan12,Yogendra"
	defaultFleetAccountTypes = "manufactura"
)

// csvSet parses a comma-separated env var into a lowercased lookup set, falling
// back to the supplied default when unset.
func csvSet(envKey, fallback string) map[string]bool {
	raw := os.Getenv(envKey)
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}
	set := make(map[string]bool)
	for _, item := range strings.Split(raw, ",") {
		if v := strings.ToLower(strings.TrimSpace(item)); v != "" {
			set[v] = true
		}
	}
	return set
}

// hasFleetAccess reports whether an account may read every machine.
// Override with FLEET_ACCESS_USERNAMES and FLEET_ACCESS_ACCOUNT_TYPES.
func hasFleetAccess(username, accountType string) bool {
	if csvSet("FLEET_ACCESS_USERNAMES", defaultFleetUsernames)[strings.ToLower(strings.TrimSpace(username))] {
		return true
	}
	return csvSet("FLEET_ACCESS_ACCOUNT_TYPES", defaultFleetAccountTypes)[strings.ToLower(strings.TrimSpace(accountType))]
}

var (
	grantCache   = make(map[int]machineGrant)
	grantCacheMu sync.RWMutex
)

// grantCacheTTL keeps the hot path off the database without letting a revoked
// grant linger. Access changes take effect within this window; a password change
// or logout is immediate because it invalidates the session itself.
const grantCacheTTL = 30 * time.Second

// InvalidateMachineGrant drops a cached grant so an admin edit applies at once.
func InvalidateMachineGrant(userID int) {
	grantCacheMu.Lock()
	delete(grantCache, userID)
	grantCacheMu.Unlock()
}

func loadMachineGrant(ctx context.Context, claims *middleware.UserClaims) (machineGrant, error) {
	grantCacheMu.RLock()
	cached, found := grantCache[claims.UserID]
	grantCacheMu.RUnlock()
	if found && time.Since(cached.loadedAt) < grantCacheTTL {
		return cached, nil
	}

	// Read the account's own row: accountType decides staff-vs-customer, and
	// monitorAccess carries a customer's explicit machine list.
	var monitorAccess, accountType sql.NullString
	err := database.SafeQueryRowContext(ctx,
		"SELECT monitorAccess, accountType FROM kabu_users WHERE id = ?", claims.UserID,
	).Scan(&monitorAccess, &accountType)
	if err != nil {
		if err == sql.ErrNoRows {
			// The account behind this token no longer exists. Deny, and let the
			// caller surface it as an authentication failure.
			return machineGrant{tables: map[string]bool{}, loadedAt: time.Now()}, nil
		}
		return machineGrant{}, err
	}

	// Fleet-wide access is decided from the database row, not from the token, so
	// revoking it does not depend on a session expiring.
	if hasFleetAccess(claims.Username, accountType.String) {
		grant := machineGrant{allAccess: true, tables: map[string]bool{}, loadedAt: time.Now()}
		grantCacheMu.Lock()
		grantCache[claims.UserID] = grant
		grantCacheMu.Unlock()
		return grant, nil
	}

	index := machineTableIndex()
	grant := machineGrant{tables: make(map[string]bool), loadedAt: time.Now()}
	for _, entry := range strings.Split(monitorAccess.String, ",") {
		if strings.TrimSpace(entry) == "" || isPlaceholderIdentifier(entry) {
			continue // "0", "devices" and friends are not machine names
		}
		key := normalizeMachineKey(entry)
		if key == "" {
			continue
		}
		if key == "*" {
			grant.allAccess = true
			continue
		}
		if table, ok := index[key]; ok {
			grant.tables[table] = true
		} else {
			log.Printf("authz: user %d has grant %q that matches no known machine", claims.UserID, entry)
		}
	}

	grantCacheMu.Lock()
	grantCache[claims.UserID] = grant
	grantCacheMu.Unlock()
	return grant, nil
}

// authorizedTables lists the physical tables a caller may read, sorted so
// behaviour (including the fallback default) is deterministic.
func authorizedTables(ctx context.Context, claims *middleware.UserClaims) ([]string, error) {
	grant, err := loadMachineGrant(ctx, claims)
	if err != nil {
		return nil, err
	}

	if grant.allAccess || !config.GetConfig().EnforceMachineScope {
		return getAllowedTables(), nil
	}

	tables := make([]string, 0, len(grant.tables))
	for table := range grant.tables {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables, nil
}

// -----------------------------------------------------------------------------
// Handler entry point
// -----------------------------------------------------------------------------

// writeAuthzError keeps one wording for "not yours" and "does not exist". The
// previous handler returned the full allowedTables list on a bad identifier,
// which handed a caller the complete machine inventory (S-07).
func writeAuthzError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": message,
	})
}

// resolveTable turns the request's `table` parameter into a physical table the
// caller is allowed to read, or writes the error response and returns false.
//
// An empty parameter resolves to the caller's own default rather than a global
// one: for a single-machine customer that is their machine, never someone
// else's.
func resolveTable(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims, ok := middleware.GetUserFromContext(r)
	if !ok {
		writeAuthzError(w, http.StatusUnauthorized, "Authentication required")
		return "", false
	}

	permitted, err := authorizedTables(r.Context(), claims)
	if err != nil {
		log.Printf("authz: could not load grants for user %d: %v", claims.UserID, err)
		writeAuthzError(w, http.StatusServiceUnavailable, "Authorization data is temporarily unavailable. Please try again.")
		return "", false
	}
	if len(permitted) == 0 {
		log.Printf("authz: user %d (%s) has no machine grants", claims.UserID, claims.Username)
		writeAuthzError(w, http.StatusForbidden, "No machines are assigned to this account.")
		return "", false
	}

	allowed := make(map[string]bool, len(permitted))
	for _, table := range permitted {
		allowed[table] = true
	}

	requested := strings.TrimSpace(r.URL.Query().Get("table"))

	// A placeholder value is a client-side bug, not an access attempt: the UI
	// forwards monitorAccess entries as the table parameter, and those entries
	// contain "0" and "devices" in the live data. Treat it as "not specified"
	// and fall through to the caller's own default. This leaks nothing — the
	// fallback can only ever be a machine this caller is already cleared for.
	if requested != "" && isPlaceholderIdentifier(requested) {
		log.Printf("authz: user %d sent placeholder table=%q; using their default machine",
			claims.UserID, requested)
		requested = ""
	}

	if requested == "" {
		return defaultTableFor(permitted), true
	}

	table, known := machineTableIndex()[normalizeMachineKey(requested)]
	if !known || !allowed[table] {
		// Detail stays in the server log; the client gets one flat answer so it
		// cannot distinguish a typo from another customer's machine.
		log.Printf("authz: DENIED user %d (%s) ip=%s table=%q known=%v",
			claims.UserID, claims.Username, middleware.ClientIP(r), requested, known)
		writeAuthzError(w, http.StatusForbidden, "You are not authorized to access the requested machine.")
		return "", false
	}

	return table, true
}

// AuthorizeTableRequest is the exported form of resolveTable, for routes wired
// as closures in main.go. It returns the physical table the caller is cleared
// for, or false after having written the error response.
func AuthorizeTableRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	return resolveTable(w, r)
}

// RequireAdmin wraps a handler so only accounts on the user-admin allowlist can
// reach it. Used for account creation and the diagnostic endpoints, which are
// staff tools rather than operator features.
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}
		claims, ok := middleware.GetUserFromContext(r)
		if !ok {
			writeAuthzError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		if !isUserAdmin(claims) {
			log.Printf("authz: DENIED admin route user %d (%s) ip=%s path=%s",
				claims.UserID, claims.Username, middleware.ClientIP(r), r.URL.Path)
			writeAuthzError(w, http.StatusForbidden, "You are not permitted to perform this action")
			return
		}
		next(w, r)
	}
}

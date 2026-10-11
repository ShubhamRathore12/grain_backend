package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"grain_backend/middleware"

	"github.com/gorilla/mux"
)

// -----------------------------------------------------------------------------
// GTPL-141 remote control adapter
//
// The browser never sees the HVAC integration token. Every route here first
// checks the Grain session, the caller's grant for GTPL-141 and (for writes) the
// control permission plus a CSRF header, and only then makes a server-to-server
// call to the HVAC control API with a freshly built Authorization header.
//
// The upstream host and the unit-141 paths are fixed. Nothing from the browser
// can choose another machine, path or host.
// -----------------------------------------------------------------------------

const (
	gtpl141Table   = "GTPL_141_GT_140E_S7_1200"
	gtpl141Unit    = "unit-141"
	gtpl141MaxBody = 2 << 20
)

// gtpl141TagType is the exact set of tags this dashboard may write. Manual_EN and
// Auto_aeration_disable are deliberately absent: they are hidden, no remote
// writes.
var gtpl141TagType = map[string]string{
	"T1_ref_from_T0":          "REAL",
	"Delta_T":                 "REAL",
	"Duration_set_aeration":   "INT",
	"Auto_aeration_enable":    "BOOL",
	"Auto_EN":                 "BOOL",
	"Set_TH_for_heating_mode": "REAL",
	"Continuous_mode":         "BOOL",
	"Set_Run_Duration":        "INT",
	"HP_set_from_HMI":         "REAL",
	"Auto_start":              "PULSE",
	"Auto_stop":               "PULSE",
	"Aeration_start_with_WH":  "PULSE",
	"Aeration_stop_with_WH":   "PULSE",
	"Aeration_start_with_WOH": "PULSE",
	"Aeration_stop_with_WOH":  "PULSE",
}

// gtpl141UpstreamURL is fixed; only tests replace it with a local stub.
var gtpl141UpstreamURL = "https://primeosys.com/hvac-dashboard/api"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var gtpl141Client = &http.Client{
	Timeout: 15 * time.Second,
	// A credential-bearing request must never follow a redirect elsewhere.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// RegisterGTPL141Routes wires the adapter onto the authenticated subrouter.
func RegisterGTPL141Routes(protected *mux.Router) {
	base := "/controls/gtpl-141"
	protected.HandleFunc(base+"/access", gtpl141Access).Methods("GET", "OPTIONS")
	protected.HandleFunc(base+"/latest", gtpl141Get("/machines/"+gtpl141Unit+"/latest")).Methods("GET", "OPTIONS")
	protected.HandleFunc(base+"/capabilities", gtpl141Get("/controls/"+gtpl141Unit+"/capabilities")).Methods("GET", "OPTIONS")
	protected.HandleFunc(base+"/commands", gtpl141Get("/controls/"+gtpl141Unit+"/commands")).Methods("GET", "OPTIONS")
	protected.HandleFunc(base+"/commands", gtpl141CreateCommand).Methods("POST")
	protected.HandleFunc(base+"/commands/{id}/cancel", gtpl141Cancel).Methods("POST")
	protected.HandleFunc(base+"/commands/{id}/review", gtpl141Review).Methods("POST")
}

func gtpl141JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func gtpl141Error(w http.ResponseWriter, status int, code, detail string) {
	gtpl141JSON(w, status, map[string]interface{}{"detail": detail, "code": code})
}

// gtpl141Authorize checks session + machine grant, and for writes the control
// permission. It writes the error response itself.
func gtpl141Authorize(w http.ResponseWriter, r *http.Request, write bool) (*middleware.UserClaims, bool) {
	claims, ok := middleware.GetUserFromContext(r)
	if !ok {
		gtpl141Error(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return nil, false
	}
	tables, err := authorizedTables(r.Context(), claims)
	if err != nil {
		gtpl141Error(w, http.StatusServiceUnavailable, "authz_unavailable", "Authorization data is temporarily unavailable.")
		return nil, false
	}
	hasMachine := false
	for _, t := range tables {
		if t == gtpl141Table {
			hasMachine = true
			break
		}
	}
	if !hasMachine {
		log.Printf("gtpl141: DENIED user %d (%s) no machine grant", claims.UserID, claims.Username)
		gtpl141Error(w, http.StatusForbidden, "forbidden", "You are not authorized to access the requested machine.")
		return nil, false
	}
	if !write {
		return claims, true
	}
	if !gtpl141CanControl(claims) {
		log.Printf("gtpl141: DENIED control user %d (%s)", claims.UserID, claims.Username)
		gtpl141Error(w, http.StatusForbidden, "control_forbidden", "Your account is not permitted to control GTPL-141.")
		return nil, false
	}
	// CSRF: the session cookie is SameSite=None, so a write must carry a custom
	// header (forcing a CORS preflight) and, when the browser sends an Origin,
	// it must be on the allowlist.
	if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
		gtpl141Error(w, http.StatusForbidden, "csrf", "Missing request header.")
		return nil, false
	}
	if origin := r.Header.Get("Origin"); origin != "" && !middleware.IsOriginAllowed(origin) {
		gtpl141Error(w, http.StatusForbidden, "csrf", "Origin not allowed.")
		return nil, false
	}
	return claims, true
}

// gtpl141CanControl: GTPL141_CONTROL_USERNAMES (CSV) restricts control to those
// accounts. When unset, every account with a GTPL-141 grant may control it.
func gtpl141CanControl(claims *middleware.UserClaims) bool {
	if strings.TrimSpace(os.Getenv("GTPL141_CONTROL_USERNAMES")) == "" {
		return true
	}
	return csvSet("GTPL141_CONTROL_USERNAMES", "")[strings.ToLower(strings.TrimSpace(claims.Username))]
}

func gtpl141Access(w http.ResponseWriter, r *http.Request) {
	claims, ok := gtpl141Authorize(w, r, false)
	if !ok {
		return
	}
	gtpl141JSON(w, http.StatusOK, map[string]interface{}{
		"can_control": gtpl141CanControl(claims),
		"configured":  strings.TrimSpace(os.Getenv("GTPL141_HVAC_TOKEN")) != "",
	})
}

// gtpl141Upstream performs the server-to-server call and relays a sanitized
// result. Only bearer auth and JSON headers are sent; nothing from the browser
// request is forwarded.
func gtpl141Upstream(w http.ResponseWriter, r *http.Request, method, path string, body []byte) (int, []byte) {
	token := strings.TrimSpace(os.Getenv("GTPL141_HVAC_TOKEN"))
	if token == "" {
		gtpl141Error(w, http.StatusServiceUnavailable, "integration_not_configured", "GTPL-141 control integration is not configured on the server.")
		return 0, nil
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, gtpl141UpstreamURL+path, reader)
	if err != nil {
		gtpl141Error(w, http.StatusInternalServerError, "internal", "Could not build upstream request.")
		return 0, nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := gtpl141Client.Do(req)
	if err != nil {
		log.Printf("gtpl141: upstream %s %s failed: %v", method, path, err)
		gtpl141JSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"code":   "upstream_unreachable",
			"detail": "HVAC control service unreachable or timed out. A write may or may not have been delivered; check command history.",
		})
		return 0, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, gtpl141MaxBody))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// Map to 502 so the browser does not treat it as the user's session
		// expiring. It is the integration credential or scope that failed.
		log.Printf("gtpl141: upstream %s %s returned %d", method, path, resp.StatusCode)
		detail := "HVAC integration authentication failed (token missing, expired or revoked)."
		if resp.StatusCode == http.StatusForbidden {
			detail = "HVAC integration refused the request (scope or tag not permitted)."
		}
		gtpl141JSON(w, http.StatusBadGateway, map[string]interface{}{
			"code": "integration_auth", "upstream_status": resp.StatusCode, "detail": detail,
		})
		return resp.StatusCode, nil
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		gtpl141Error(w, http.StatusBadGateway, "upstream_redirect", "Unexpected upstream redirect.")
		return resp.StatusCode, nil
	}

	status := resp.StatusCode
	if status >= 500 {
		status = http.StatusServiceUnavailable
	}
	if !json.Valid(raw) {
		gtpl141Error(w, status, "upstream_invalid", fmt.Sprintf("HVAC control service returned HTTP %d.", resp.StatusCode))
		return resp.StatusCode, nil
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(raw)
	return resp.StatusCode, raw
}

func gtpl141Get(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := gtpl141Authorize(w, r, false); !ok {
			return
		}
		gtpl141Upstream(w, r, http.MethodGet, path, nil)
	}
}

// decodeStrict reads a small JSON body and rejects unknown fields.
func decodeStrict(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}

// validateValue checks a JSON value against the tag's PLC storage type and
// returns the normalized value. These are storage limits only.
func validateValue(kind string, raw json.RawMessage) (interface{}, error) {
	trimmed := bytes.TrimSpace(raw)
	switch kind {
	case "BOOL", "PULSE":
		var b bool
		if err := json.Unmarshal(trimmed, &b); err != nil {
			return nil, errors.New("must be JSON true or false")
		}
		return b, nil
	case "INT":
		var n json.Number
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&n); err != nil {
			return nil, errors.New("must be a whole number")
		}
		i, err := n.Int64()
		if err != nil || i < -32768 || i > 32767 {
			return nil, errors.New("must be a whole number within -32768..32767")
		}
		return i, nil
	case "REAL":
		var f float64
		if err := json.Unmarshal(trimmed, &f); err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, errors.New("must be a finite number")
		}
		if math.Abs(f) > math.MaxFloat32 {
			return nil, errors.New("is outside PLC REAL range")
		}
		return f, nil
	}
	return nil, errors.New("unknown tag")
}

func gtpl141CreateCommand(w http.ResponseWriter, r *http.Request) {
	claims, ok := gtpl141Authorize(w, r, true)
	if !ok {
		return
	}
	var in struct {
		RequestID     string          `json:"request_id"`
		Tag           string          `json:"tag"`
		Value         json.RawMessage `json:"value"`
		ExpectedValue json.RawMessage `json:"expected_value"`
	}
	if err := decodeStrict(r, &in); err != nil {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "Invalid command body.")
		return
	}
	if !uuidPattern.MatchString(in.RequestID) {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "request_id must be a UUID.")
		return
	}
	kind, known := gtpl141TagType[in.Tag]
	if !known {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "This tag cannot be written from the dashboard.")
		return
	}
	value, err := validateValue(kind, in.Value)
	if err != nil {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "value "+err.Error())
		return
	}
	expected, err := validateValue(kind, in.ExpectedValue)
	if err != nil {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "expected_value "+err.Error())
		return
	}
	if kind == "PULSE" && (value != true || expected != false) {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "A pulse must request true from idle false.")
		return
	}

	// The reason is derived from the verified principal so it stays identical
	// on retries of the same request_id.
	reason := fmt.Sprintf("Grain Technik dashboard command; operator=%d:%s", claims.UserID, claims.Username)
	if len(reason) > 300 {
		reason = reason[:300]
	}
	body, _ := json.Marshal(map[string]interface{}{
		"request_id":     strings.ToLower(in.RequestID),
		"tag":            in.Tag,
		"value":          value,
		"expected_value": expected,
		"mode":           "live",
		"reason":         reason,
	})

	log.Printf("AUDIT gtpl141 submit user=%d username=%q ip=%s request_id=%s tag=%s value=%v expected=%v",
		claims.UserID, claims.Username, middleware.ClientIP(r), in.RequestID, in.Tag, value, expected)
	status, raw := gtpl141Upstream(w, r, http.MethodPost, "/controls/"+gtpl141Unit+"/commands", body)
	var out struct {
		ID     string      `json:"id"`
		Status string      `json:"status"`
		Detail interface{} `json:"detail"`
	}
	json.Unmarshal(raw, &out)
	log.Printf("AUDIT gtpl141 result user=%d request_id=%s tag=%s http=%d command=%s status=%s detail=%v",
		claims.UserID, in.RequestID, in.Tag, status, out.ID, out.Status, out.Detail)
}

func gtpl141CommandID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := mux.Vars(r)["id"]
	if !uuidPattern.MatchString(id) {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "Invalid command id.")
		return "", false
	}
	return strings.ToLower(id), true
}

func gtpl141Cancel(w http.ResponseWriter, r *http.Request) {
	claims, ok := gtpl141Authorize(w, r, true)
	if !ok {
		return
	}
	id, ok := gtpl141CommandID(w, r)
	if !ok {
		return
	}
	status, _ := gtpl141Upstream(w, r, http.MethodPost, "/controls/"+gtpl141Unit+"/commands/"+id+"/cancel", []byte("{}"))
	log.Printf("AUDIT gtpl141 cancel user=%d username=%q command=%s http=%d", claims.UserID, claims.Username, id, status)
}

func gtpl141Review(w http.ResponseWriter, r *http.Request) {
	claims, ok := gtpl141Authorize(w, r, true)
	if !ok {
		return
	}
	id, ok := gtpl141CommandID(w, r)
	if !ok {
		return
	}
	var in struct {
		PlcInspected bool   `json:"plc_inspected"`
		Outcome      string `json:"outcome"`
	}
	if err := decodeStrict(r, &in); err != nil || !in.PlcInspected ||
		(in.Outcome != "requested_change_confirmed" && in.Outcome != "different_state_confirmed") {
		gtpl141Error(w, http.StatusUnprocessableEntity, "invalid", "Review requires a confirmed PLC/HMI inspection and a finding.")
		return
	}
	body, _ := json.Marshal(map[string]interface{}{"plc_inspected": true, "outcome": in.Outcome})
	status, _ := gtpl141Upstream(w, r, http.MethodPost, "/controls/"+gtpl141Unit+"/commands/"+id+"/review", body)
	log.Printf("AUDIT gtpl141 review user=%d username=%q command=%s outcome=%s http=%d",
		claims.UserID, claims.Username, id, in.Outcome, status)
}

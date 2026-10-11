package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grain_backend/middleware"

	"github.com/gorilla/mux"
)

// stubUpstream records every call; no real HVAC endpoint is ever contacted.
func stubUpstream(t *testing.T, status int, body string) (*[]*http.Request, *[]string) {
	var reqs []*http.Request
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		reqs = append(reqs, r)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := gtpl141UpstreamURL
	gtpl141UpstreamURL = srv.URL
	t.Cleanup(func() { gtpl141UpstreamURL = old })
	t.Setenv("GTPL141_HVAC_TOKEN", "test-token")
	return &reqs, &bodies
}

func grantUser(id int, all bool, tables ...string) *middleware.UserClaims {
	g := machineGrant{allAccess: all, tables: map[string]bool{}, loadedAt: time.Now().Add(time.Hour)}
	for _, tb := range tables {
		g.tables[tb] = true
	}
	grantCacheMu.Lock()
	grantCache[id] = g
	grantCacheMu.Unlock()
	return &middleware.UserClaims{UserID: id, Username: "op" + string(rune('0'+id))}
}

func call(claims *middleware.UserClaims, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := mux.NewRouter()
	RegisterGTPL141Routes(r.PathPrefix("/api").Subrouter())
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Cookie", "auth_token=browser-secret")
	req.Header.Set("Origin", "http://localhost:3000")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if claims != nil {
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, claims))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var xhr = map[string]string{"X-Requested-With": "XMLHttpRequest"}

const validCmd = `{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Set_Run_Duration","value":13,"expected_value":12}`

func TestGTPL141ReadForwardsOnlyBearer(t *testing.T) {
	reqs, _ := stubUpstream(t, 200, `{"mode":"live"}`)
	u := grantUser(1, false, gtpl141Table)
	w := call(u, "GET", "/api/controls/gtpl-141/capabilities", "", nil)
	if w.Code != 200 || len(*reqs) != 1 {
		t.Fatalf("code=%d calls=%d", w.Code, len(*reqs))
	}
	up := (*reqs)[0]
	if up.URL.Path != "/controls/unit-141/capabilities" || up.Header.Get("Authorization") != "Bearer test-token" {
		t.Fatalf("bad upstream %s %q", up.URL.Path, up.Header.Get("Authorization"))
	}
	if up.Header.Get("Cookie") != "" || up.Header.Get("Origin") != "" || up.Header.Get("Sec-Fetch-Site") != "" {
		t.Fatal("browser headers forwarded")
	}
	if strings.Contains(w.Body.String(), "test-token") {
		t.Fatal("token leaked")
	}
}

func TestGTPL141OtherMachineUserDenied(t *testing.T) {
	reqs, _ := stubUpstream(t, 200, `{}`)
	u := grantUser(2, false, "GTPL_114_GT_140E_S7_1200")
	for _, p := range []string{"/latest", "/capabilities", "/commands"} {
		if w := call(u, "GET", "/api/controls/gtpl-141"+p, "", nil); w.Code != 403 {
			t.Fatalf("%s code=%d", p, w.Code)
		}
	}
	if w := call(u, "POST", "/api/controls/gtpl-141/commands", validCmd, xhr); w.Code != 403 {
		t.Fatalf("post code=%d", w.Code)
	}
	if len(*reqs) != 0 {
		t.Fatal("upstream called for unauthorized user")
	}
}

func TestGTPL141CreateBuildsServerSideBody(t *testing.T) {
	reqs, bodies := stubUpstream(t, 200, `{"id":"x","status":"queued"}`)
	u := grantUser(3, false, gtpl141Table)
	w := call(u, "POST", "/api/controls/gtpl-141/commands", validCmd, xhr)
	if w.Code != 200 || len(*reqs) != 1 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var sent map[string]interface{}
	json.Unmarshal([]byte((*bodies)[0]), &sent)
	if sent["mode"] != "live" || sent["value"].(float64) != 13 || !strings.Contains(sent["reason"].(string), "operator=3:") {
		t.Fatalf("bad body %v", sent)
	}
	if (*reqs)[0].Header.Get("Content-Type") != "application/json" {
		t.Fatal("missing content type")
	}
}

func TestGTPL141RejectsBadCommands(t *testing.T) {
	reqs, _ := stubUpstream(t, 200, `{}`)
	u := grantUser(4, false, gtpl141Table)
	bad := []string{
		`{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Manual_EN","value":true,"expected_value":false}`,
		`{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Auto_aeration_disable","value":true,"expected_value":false}`,
		`{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Set_Run_Duration","value":13.5,"expected_value":12}`,
		`{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Set_Run_Duration","value":40000,"expected_value":12}`,
		`{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Auto_EN","value":1,"expected_value":0}`,
		`{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Auto_start","value":false,"expected_value":true}`,
		`{"request_id":"not-a-uuid","tag":"Auto_start","value":true,"expected_value":false}`,
		`{"request_id":"3f1c2a4e-1b2c-4d5e-8f90-123456789abc","tag":"Auto_start","value":true,"expected_value":false,"mode":"live"}`,
	}
	for _, b := range bad {
		if w := call(u, "POST", "/api/controls/gtpl-141/commands", b, xhr); w.Code != 422 {
			t.Fatalf("expected 422 for %s got %d", b, w.Code)
		}
	}
	if len(*reqs) != 0 {
		t.Fatal("invalid command reached upstream")
	}
}

func TestGTPL141CSRFAndControlPermission(t *testing.T) {
	reqs, _ := stubUpstream(t, 200, `{}`)
	u := grantUser(5, false, gtpl141Table)
	if w := call(u, "POST", "/api/controls/gtpl-141/commands", validCmd, nil); w.Code != 403 {
		t.Fatalf("no xhr header code=%d", w.Code)
	}
	if w := call(u, "POST", "/api/controls/gtpl-141/commands", validCmd,
		map[string]string{"X-Requested-With": "XMLHttpRequest", "Origin": "https://evil.example"}); w.Code != 403 {
		t.Fatalf("bad origin code=%d", w.Code)
	}
	t.Setenv("GTPL141_CONTROL_USERNAMES", "someoneelse")
	if w := call(u, "POST", "/api/controls/gtpl-141/commands", validCmd, xhr); w.Code != 403 {
		t.Fatalf("control perm code=%d", w.Code)
	}
	if len(*reqs) != 0 {
		t.Fatal("upstream called")
	}
}

func TestGTPL141UpstreamAuthFailureIsNot401(t *testing.T) {
	stubUpstream(t, 401, `{"detail":"bad token"}`)
	u := grantUser(6, false, gtpl141Table)
	w := call(u, "GET", "/api/controls/gtpl-141/latest", "", nil)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "integration_auth") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGTPL141ConflictPassesThrough(t *testing.T) {
	stubUpstream(t, 409, `{"detail":"expected_value does not match"}`)
	u := grantUser(7, false, gtpl141Table)
	w := call(u, "POST", "/api/controls/gtpl-141/commands", validCmd, xhr)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "expected_value") {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestGTPL141ReviewAndCancel(t *testing.T) {
	reqs, bodies := stubUpstream(t, 200, `{}`)
	u := grantUser(8, false, gtpl141Table)
	id := "3f1c2a4e-1b2c-4d5e-8f90-123456789abc"
	if w := call(u, "POST", "/api/controls/gtpl-141/commands/"+id+"/review", `{"plc_inspected":true,"outcome":"unable_to_confirm"}`, xhr); w.Code != 422 {
		t.Fatalf("unable_to_confirm must not be submitted, code=%d", w.Code)
	}
	if w := call(u, "POST", "/api/controls/gtpl-141/commands/"+id+"/review", `{"plc_inspected":false,"outcome":"different_state_confirmed"}`, xhr); w.Code != 422 {
		t.Fatalf("uninspected review code=%d", w.Code)
	}
	if w := call(u, "POST", "/api/controls/gtpl-141/commands/../../machines/cancel", `{}`, xhr); w.Code == 200 {
		t.Fatal("path traversal accepted")
	}
	call(u, "POST", "/api/controls/gtpl-141/commands/"+id+"/cancel", `{}`, xhr)
	if len(*reqs) != 1 || (*reqs)[0].URL.Path != "/controls/unit-141/commands/"+id+"/cancel" || (*bodies)[0] != "{}" {
		t.Fatalf("cancel upstream wrong: %d", len(*reqs))
	}
}

func TestGTPL141NotConfigured(t *testing.T) {
	t.Setenv("GTPL141_HVAC_TOKEN", "")
	u := grantUser(9, false, gtpl141Table)
	if w := call(u, "GET", "/api/controls/gtpl-141/latest", "", nil); w.Code != 503 {
		t.Fatalf("code=%d", w.Code)
	}
}

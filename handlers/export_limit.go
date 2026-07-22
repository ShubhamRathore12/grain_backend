package handlers

import (
	"net/http"
)

// maxConcurrentExports bounds how many full-table-scan exports run at once.
// Exports are heavy (they stream whole tables in batches, each holding a DB
// connection for the duration), so without a cap a handful of concurrent
// exports can drain the connection pool and starve every other endpoint.
const maxConcurrentExports = 3

var exportSem = make(chan struct{}, maxConcurrentExports)

// acquireExportSlot tries to reserve a global export slot. On success it returns
// a release function that MUST be deferred by the caller. On failure it writes a
// 429 response and returns false — the caller should return immediately.
func acquireExportSlot(w http.ResponseWriter) (release func(), ok bool) {
	select {
	case exportSem <- struct{}{}:
		return func() { <-exportSem }, true
	default:
		w.Header().Set("Retry-After", "30")
		http.Error(w, `{"error": "Server busy: too many exports in progress, retry shortly"}`, http.StatusTooManyRequests)
		return nil, false
	}
}

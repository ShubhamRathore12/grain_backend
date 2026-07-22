package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"grain_backend/database"
)

// HandleGetAllData handles getting all data from specified table
func HandleGetAllData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	table := r.URL.Query().Get("table")
	if table == "" {
		table = "kabomachinedatasmart200"
	}

	// Validate table name (whitelist approach)
	allowedTables := getAllowedTables()
	if !contains(allowedTables, table) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":       false,
			"error":         "Invalid table name",
			"allowedTables": allowedTables,
		})
		return
	}

	query := "SELECT * FROM `" + table + "` ORDER BY id DESC LIMIT 1"
	rows, err := database.SafeQueryContext(r.Context(), query)
	if err != nil {
		log.Printf("Error fetching data: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to fetch data",
			"message": err.Error(),
		})
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		http.Error(w, `{"error": "Database result error"}`, http.StatusInternalServerError)
		return
	}
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	var result map[string]interface{}
	if rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			http.Error(w, `{"error": "Database result error"}`, http.StatusInternalServerError)
			return
		}
		result = make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				result[col] = string(b)
			} else if t, ok := val.(time.Time); ok {
				result[col] = t.Format("2006-01-02 15:04:05")
			} else {
				result[col] = val
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"data":      result,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleGetPaginatedData handles paginated data retrieval
func HandleGetPaginatedData(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}
	if limit > 1000 {
		limit = 1000
	}

	// Support both "from"/"to" and "fromDate"/"toDate" query params
	fromDate := r.URL.Query().Get("from")
	if fromDate == "" {
		fromDate = r.URL.Query().Get("fromDate")
	}
	toDate := r.URL.Query().Get("to")
	if toDate == "" {
		toDate = r.URL.Query().Get("toDate")
	}
	fromDate, toDate = normalizeDateRange(fromDate, toDate)
	table := r.URL.Query().Get("table")
	if table == "" {
		table = "kabomachinedatasmart200"
	}

	allowedTables := getAllowedTables()
	if !contains(allowedTables, table) {
		http.Error(w, `{"error": "Invalid table name"}`, http.StatusBadRequest)
		return
	}

	offset := (page - 1) * limit

	// Detect timestamp column for this table
	tsCol := getTimestampColumn(r.Context(), table)

	// Build WHERE clause. Skip date filter when no timestamp column exists
	// to avoid "Unknown column" SQL errors on tables without one.
	whereClause := ""
	params := []interface{}{}
	if tsCol != "" && (fromDate != "" || toDate != "") {
		conditions := []string{}
		if fromDate != "" {
			conditions = append(conditions, "`"+tsCol+"` >= ?")
			params = append(params, fromDate)
		}
		if toDate != "" {
			conditions = append(conditions, "`"+tsCol+"` <= ?")
			params = append(params, toDate)
		}
		if len(conditions) > 0 {
			whereClause = "WHERE " + joinStrings(conditions, " AND ")
		}
	}

	// Get total count
	countQuery := "SELECT COUNT(*) as total FROM `" + table + "` " + whereClause
	var total int
	if err := database.SafeQueryRowContext(r.Context(), countQuery, params...).Scan(&total); err != nil {
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}

	// Get paginated data
	dataParams := append(append([]interface{}{}, params...), limit, offset)
	dataQuery := "SELECT * FROM `" + table + "` " + whereClause + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := database.SafeQueryContext(r.Context(), dataQuery, dataParams...)
	if err != nil {
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	data := scanRowsToMap(rows)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":       data,
		"page":       page,
		"limit":      limit,
		"total":      total,
		"totalPages": (total + limit - 1) / limit,
	})
}

// Helper functions
// getAllowedTables returns the whitelist of queryable tables. It derives from
// machineStatusTables so the whitelist and the status dashboard can never drift.
// Sorted for a deterministic response order.
func getAllowedTables() []string {
	tables := make([]string, 0, len(machineStatusTables))
	for tableName := range machineStatusTables {
		tables = append(tables, tableName)
	}
	sort.Strings(tables)
	return tables
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// tsColumnCache caches detected timestamp column per table to avoid repeated INFORMATION_SCHEMA queries.
var tsColumnCache sync.Map

// getTimestampColumn detects the timestamp column name for a given table.
// Filters by current DATABASE() schema and orders results so preferred names win deterministically.
// Result is cached per-table.
func getTimestampColumn(ctx context.Context, table string) string {
	if cached, ok := tsColumnCache.Load(table); ok {
		return cached.(string)
	}

	// FIELD() returns position in the list (1 = highest priority); columns not in the list get 0.
	// Filtering by TABLE_SCHEMA = DATABASE() avoids cross-DB collisions.
	query := `SELECT COLUMN_NAME
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND COLUMN_NAME IN ('created_at','created_on','CreatedAt','CreatedOn','timestamp','Timestamp','DateTime','datetime','date_time','Date','date','time')
		ORDER BY FIELD(COLUMN_NAME,'created_at','created_on','CreatedAt','CreatedOn','timestamp','Timestamp','DateTime','datetime','date_time','Date','date','time')
		LIMIT 1`
	col := ""
	err := database.SafeQueryRowContext(ctx, query, table).Scan(&col)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("getTimestampColumn error for %s: %v", table, err)
		// Don't cache the default — let the next call retry detection.
		return "created_at"
	}
	if col == "" {
		// No matching timestamp column exists in this table. Caller must handle this.
		log.Printf("getTimestampColumn: no timestamp column found for %s", table)
		tsColumnCache.Store(table, "")
		return ""
	}
	tsColumnCache.Store(table, col)
	return col
}

// normalizeDateRange expands date-only inputs to full-day boundaries.
// "2025-01-03" as toDate becomes "2025-01-03 23:59:59" so the entire day is included.
// "2025-01-01" as fromDate becomes "2025-01-01 00:00:00" for symmetry.
// Inputs that already include a time component are returned unchanged.
func normalizeDateRange(fromDate, toDate string) (string, string) {
	if fromDate != "" && len(fromDate) == 10 {
		fromDate = fromDate + " 00:00:00"
	}
	if toDate != "" && len(toDate) == 10 {
		toDate = toDate + " 23:59:59"
	}
	return fromDate, toDate
}

func scanRowsToMap(rows *sql.Rows) []map[string]interface{} {
	result := []map[string]interface{}{}
	columns, _ := rows.Columns()

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		rows.Scan(valuePtrs...)
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else if t, ok := val.(time.Time); ok {
				rowMap[col] = t.Format("2006-01-02 15:04:05")
			} else {
				rowMap[col] = val
			}
		}
		result = append(result, rowMap)
	}

	return result
}

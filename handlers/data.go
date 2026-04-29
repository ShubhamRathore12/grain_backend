package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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
	rows, err := database.SafeQuery(query)
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

	columns, _ := rows.Columns()
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	var result map[string]interface{}
	if rows.Next() {
		rows.Scan(valuePtrs...)
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

	// Support both "from"/"to" and "fromDate"/"toDate" query params
	fromDate := r.URL.Query().Get("from")
	if fromDate == "" {
		fromDate = r.URL.Query().Get("fromDate")
	}
	toDate := r.URL.Query().Get("to")
	if toDate == "" {
		toDate = r.URL.Query().Get("toDate")
	}
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
	tsCol := getTimestampColumn(table)

	// Build WHERE clause
	whereClause := ""
	params := []interface{}{}
	if fromDate != "" || toDate != "" {
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
	countRows, err := database.SafeQuery(countQuery, params...)
	if err != nil {
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	defer countRows.Close()

	var total int
	if countRows.Next() {
		countRows.Scan(&total)
	}

	// Get paginated data
	dataParams := append(params, limit, offset)
	dataQuery := "SELECT * FROM `" + table + "` " + whereClause + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := database.SafeQuery(dataQuery, dataParams...)
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
func getAllowedTables() []string {
	return []string{
		"GTPL_108_gT_40E_P_S7_200_Germany",
		"GTPL_109_gT_40E_P_S7_200_Germany",
		"GTPL_110_gT_40E_P_S7_200_Germany",
		"GTPL_111_gT_80E_P_S7_200_Germany",
		"GTPL_112_gT_80E_P_S7_200_Germany",
		"GTPL_113_gT_80E_P_S7_200_Germany",
		"kabomachinedatasmart200",
		"GTPL_114_GT_140E_S7_1200",
		"GTPL_115_GT_180E_S7_1200",
		"GTPL_119_GT_180E_S7_1200",
		"GTPL_120_GT_180E_S7_1200",
		"GTPL_116_GT_240E_S7_1200",
		"GTPL_117_GT_320E_S7_1200",
		"GTPL_121_GT1000T",
		"gtpl_122_s7_1200_01",
		"GTPL_124_GT_450T_S7_1200",
		"GTPL_133_GT_650T_S7_1200",
		"GTPL_131_GT_650T_S7_1200",
		"GTPL_132_GT300AP",
		"GTPL_137_GT_450T_S7_1200",
		"GTPL_138_GT_450T_S7_1200",
		"GTPL_136_GT_450AP_S7_1200",
		"GTPL_134_GT_450T_S7_1200",
		"GTPL_135_GT_450T_S7_1200",
		"GTPL_061_GT_450T_S7_1200",
		"GTPL_139_GT300AP",
		"GTPL_142_GT_450AP_S7_1200",
		"GTPL_123_GT_450AP_S7_1200",
		"GTPL_143_GT_450AP_S7_1200",
		"GTPL_145_GT_450T_S7_1200",
		"GTPL_148_GT_450T_S7_1200",
		"GTPL_144_GT_300AP_S7_1200",
		"GTPL_118_GT_60T_S7_1200",
	}
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

// getTimestampColumn detects the timestamp column name for a given table
func getTimestampColumn(table string) string {
	query := "SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = ? AND COLUMN_NAME IN ('created_at', 'created_on', 'CreatedAt', 'CreatedOn', 'timestamp', 'Timestamp', 'DateTime', 'datetime', 'date_time', 'Date', 'date', 'time') LIMIT 1"
	rows, err := database.SafeQuery(query, table)
	if err != nil {
		return "created_at"
	}
	defer rows.Close()

	if rows.Next() {
		var col string
		rows.Scan(&col)
		return col
	}
	return "created_at"
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

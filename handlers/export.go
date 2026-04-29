package handlers

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"time"

	"grain_backend/database"
)

// HandleExportCSV exports table data as a CSV file (Excel-compatible)
// Query params: table, fromDate, toDate (defaults to last 3 days)
func HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
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

	// Default to last 3 days if no dates provided
	fromDate := r.URL.Query().Get("fromDate")
	toDate := r.URL.Query().Get("toDate")

	if fromDate == "" {
		fromDate = time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	}
	if toDate == "" {
		toDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	}

	// Build query with date filter
	query := "SELECT * FROM `" + table + "` WHERE created_at >= ? AND created_at <= ? ORDER BY id DESC"
	rows, err := database.SafeQuery(query, fromDate, toDate)
	if err != nil {
		log.Printf("Export error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Printf("Export columns error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}

	// Set headers for CSV download
	filename := fmt.Sprintf("%s_%s_to_%s.csv", table, fromDate, toDate)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	// BOM for Excel to recognize UTF-8
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header row
	writer.Write(columns)

	// Write data rows
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		rows.Scan(valuePtrs...)
		row := make([]string, len(columns))
		for i, val := range values {
			switch v := val.(type) {
			case []byte:
				row[i] = string(v)
			case time.Time:
				row[i] = v.Format("2006-01-02 15:04:05")
			case nil:
				row[i] = ""
			default:
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		writer.Write(row)
	}
}

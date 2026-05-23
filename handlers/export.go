package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"grain_backend/database"
)

const exportBatchSize = 5000

// machineTimezones maps table name prefixes to their timezone locations
// Based on machine installation locations
var machineTimezones = map[string]*time.Location{
	// Germany (GMT+2)
	"GTPL_108": mustLoadLocation("Europe/Berlin"),
	"GTPL_109": mustLoadLocation("Europe/Berlin"),
	"GTPL_110": mustLoadLocation("Europe/Berlin"),
	"GTPL_111": mustLoadLocation("Europe/Berlin"),
	"GTPL_112": mustLoadLocation("Europe/Berlin"),
	"GTPL_113": mustLoadLocation("Europe/Berlin"),
	"GTPL_115": mustLoadLocation("Europe/Berlin"),
	"GTPL_116": mustLoadLocation("Europe/Berlin"),
	"GTPL_117": mustLoadLocation("Europe/Berlin"),
	"GTPL_119": mustLoadLocation("Europe/Berlin"),
	"GTPL_120": mustLoadLocation("Europe/Berlin"),
	"GTPL_030": mustLoadLocation("Europe/Berlin"),

	// Türkiye (GMT+3)
	"GTPL_061": mustLoadLocation("Europe/Istanbul"),

	// India (GMT+5:30)
	"GTPL_081": mustLoadLocation("Asia/Kolkata"),
	"GTPL_105": mustLoadLocation("Asia/Kolkata"),
	"GTPL_118": mustLoadLocation("Asia/Kolkata"),
	"GTPL_121": mustLoadLocation("Asia/Kolkata"),
	"GTPL_122": mustLoadLocation("Asia/Kolkata"),
	"GTPL_123": mustLoadLocation("Asia/Kolkata"),
	"GTPL_132": mustLoadLocation("Asia/Kolkata"),
	"GTPL_133": mustLoadLocation("Asia/Kolkata"),
	"GTPL_134": mustLoadLocation("Asia/Kolkata"),
	"GTPL_135": mustLoadLocation("Asia/Kolkata"),
	"GTPL_139": mustLoadLocation("Asia/Kolkata"),
	"GTPL_142": mustLoadLocation("Asia/Kolkata"),
	"GTPL_143": mustLoadLocation("Asia/Kolkata"),
	"GTPL_144": mustLoadLocation("Asia/Kolkata"),
	"GTPL_145": mustLoadLocation("Asia/Kolkata"),
	"GTPL_148": mustLoadLocation("Asia/Kolkata"),

	// Sri Lanka (UTC+5:30 same as IST)
	"GTPL_136": mustLoadLocation("Asia/Colombo"),

	// Indonesia (GMT+7)
	"GTPL_124": mustLoadLocation("Asia/Jakarta"),

	// Thailand (GMT+7)
	"GTPL_137": mustLoadLocation("Asia/Bangkok"),
	"GTPL_138": mustLoadLocation("Asia/Bangkok"),

	// Default for kabo
	"kabo": mustLoadLocation("Asia/Kolkata"),
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Fallback to fixed offset if timezone DB not available
		switch name {
		case "Europe/Berlin":
			return time.FixedZone("CET", 2*60*60)
		case "Europe/Istanbul":
			return time.FixedZone("TRT", 3*60*60)
		case "Asia/Kolkata":
			return time.FixedZone("IST", 5*60*60+30*60)
		case "Asia/Colombo":
			return time.FixedZone("SLT", 5*60*60+30*60)
		case "Asia/Jakarta":
			return time.FixedZone("WIB", 7*60*60)
		case "Asia/Bangkok":
			return time.FixedZone("ICT", 7*60*60)
		default:
			return time.UTC
		}
	}
	return loc
}

// getTimezoneForTable returns the timezone for a given table name
func getTimezoneForTable(table string) *time.Location {
	// Extract machine prefix like "GTPL_145" from table name like "GTPL_145_GT_450T_S7_1200"
	parts := strings.Split(table, "_")
	if len(parts) >= 2 {
		prefix := parts[0] + "_" + parts[1]
		if loc, ok := machineTimezones[prefix]; ok {
			return loc
		}
	}

	// Check lowercase prefix (e.g., "gtpl_122_s7_1200_01")
	lower := strings.ToLower(table)
	if strings.HasPrefix(lower, "gtpl_") && len(parts) >= 2 {
		prefix := "GTPL_" + parts[1]
		if loc, ok := machineTimezones[prefix]; ok {
			return loc
		}
	}

	// Check kabo
	if strings.HasPrefix(lower, "kabo") {
		return machineTimezones["kabo"]
	}

	return time.UTC
}

// HandleExportCSV exports table data as a CSV file (Excel-compatible)
// Fetches data in batches to avoid OOM on low-memory servers
// Skips duplicate rows where data columns (excluding id/timestamp) are identical
// Converts timestamps to the machine's local timezone
// Also emails the CSV to configured receivers in the background
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
	fromDate, toDate = normalizeDateRange(fromDate, toDate)

	// Detect timestamp column for this table
	tsCol := getTimestampColumn(table)

	// Get the timezone for this machine
	machineTZ := getTimezoneForTable(table)

	// First get columns from a single row
	colQuery := "SELECT * FROM `" + table + "` LIMIT 1"
	colRows, err := database.SafeQuery(colQuery)
	if err != nil {
		log.Printf("Export columns error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	allColumns, err := colRows.Columns()
	colRows.Close()
	if err != nil {
		log.Printf("Export columns error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}

	// Determine which columns to export (filter for AP models)
	columns := allColumns
	colSourceIndices := make([]int, len(allColumns)) // maps export col index -> original col index
	for i := range allColumns {
		colSourceIndices[i] = i
	}
	if isAPModel(table) {
		filtered, indices := filterColumns(allColumns, apModelColumns)
		if len(filtered) > 0 {
			columns = filtered
			colSourceIndices = indices
		}
	} else if isGTPL124(table) {
		filtered, indices := filterColumns(allColumns, gtpl124Columns)
		if len(filtered) > 0 {
			columns = filtered
			colSourceIndices = indices
		}
	} else if isThailandT(table) {
		filtered, indices := filterColumns(allColumns, thailandTColumns)
		if len(filtered) > 0 {
			columns = filtered
			colSourceIndices = indices
		}
	} else if isGTPL118(table) {
		filtered, indices := filterColumns(allColumns, gtpl118Columns)
		if len(filtered) > 0 {
			columns = filtered
			colSourceIndices = indices
		}
	} else if isEModel(table) {
		filtered, indices := filterColumns(allColumns, eModelColumns)
		if len(filtered) > 0 {
			columns = filtered
			colSourceIndices = indices
		}
	} else if isEPModel(table) {
		filtered, indices := filterColumns(allColumns, epModelColumns)
		if len(filtered) > 0 {
			columns = filtered
			colSourceIndices = indices
		}
	} else if isTModel(table) {
		filtered, indices := filterColumns(allColumns, tModelColumns)
		if len(filtered) > 0 {
			columns = filtered
			colSourceIndices = indices
		}
	}

	// Always show id, created_at, created_on first.
	columns, colSourceIndices = promoteTimestampCols(columns, colSourceIndices)

	// Identify timestamp column indices and data columns for dedup
	tsColNames := map[string]bool{
		"created_at": true, "created_on": true, "CreatedAt": true, "CreatedOn": true,
		"updated_at": true, "updated_on": true, "UpdatedAt": true, "UpdatedOn": true,
		"timestamp": true, "Timestamp": true, "DateTime": true, "datetime": true,
		"date_time": true, "Date": true, "date": true, "time": true,
	}

	skipCols := map[string]bool{
		"id": true, "ID": true, "Id": true,
	}
	for k, v := range tsColNames {
		skipCols[k] = v
	}

	// Track which column indices are timestamps (for timezone conversion)
	tsColIndices := map[int]bool{}
	dataColIndices := []int{}
	for i, col := range columns {
		if tsColNames[col] {
			tsColIndices[i] = true
		}
		if !skipCols[col] {
			dataColIndices = append(dataColIndices, i)
		}
	}

	// Set headers for CSV download
	filename := fmt.Sprintf("%s_%s_to_%s.csv", table, fromDate, toDate)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// BOM for Excel to recognize UTF-8
	bom := []byte{0xEF, 0xBB, 0xBF}

	// Write to both HTTP response and a buffer (for email attachment)
	var emailBuf bytes.Buffer
	emailBuf.Write(bom)
	w.Write(bom)

	// Create CSV writers for both response and email buffer
	multiWriter := io.MultiWriter(w, &emailBuf)
	writer := csv.NewWriter(multiWriter)

	// Write header row
	writer.Write(columns)
	writer.Flush()

	// Find the timestamp column index in the source (allColumns) for 2-min spacing
	tsSourceIdx := -1
	for i, col := range allColumns {
		if col == tsCol {
			tsSourceIdx = i
			break
		}
	}

	// Track previous row's data columns to skip duplicates
	prevDataKey := ""
	var prevTimestamp time.Time
	offset := 0

	// Fetch in batches to avoid loading everything into memory.
	// If the table has no recognized timestamp column, fall back to id-only paging.
	for {
		var query string
		var queryArgs []interface{}
		if tsCol == "" {
			query = fmt.Sprintf("SELECT * FROM `%s` ORDER BY id DESC LIMIT %d OFFSET %d",
				table, exportBatchSize, offset)
		} else {
			query = fmt.Sprintf("SELECT * FROM `%s` WHERE `%s` >= ? AND `%s` <= ? ORDER BY id DESC LIMIT %d OFFSET %d",
				table, tsCol, tsCol, exportBatchSize, offset)
			queryArgs = []interface{}{fromDate, toDate}
		}

		rows, err := database.SafeQuery(query, queryArgs...)
		if err != nil {
			log.Printf("Export batch error at offset %d: %v", offset, err)
			break
		}

		rowCount := 0
		values := make([]interface{}, len(allColumns))
		valuePtrs := make([]interface{}, len(allColumns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		for rows.Next() {
			rowCount++
			rows.Scan(valuePtrs...)

			// Extract only the export columns
			// For Indian machines, keep times as-is from database (no timezone conversion)
			isIndian := isIndianMachine(table)
			row := make([]string, len(columns))
			for ei, ai := range colSourceIndices {
				val := values[ai]
				switch v := val.(type) {
				case []byte:
					if tsColIndices[ei] {
						if t, err := time.Parse("2006-01-02 15:04:05", string(v)); err == nil {
							if isIndian {
								// Indian machines: keep time as-is from database
								row[ei] = t.Format("2006-01-02 15:04:05")
							} else {
								// Other machines: convert to machine's local timezone
								row[ei] = t.In(machineTZ).Format("2006-01-02 15:04:05")
							}
						} else {
							row[ei] = string(v)
						}
					} else {
						row[ei] = string(v)
					}
				case time.Time:
					if isIndian {
						// Indian machines: keep time as-is from database
						row[ei] = v.Format("2006-01-02 15:04:05")
					} else {
						// Other machines: convert to machine's local timezone
						row[ei] = v.In(machineTZ).Format("2006-01-02 15:04:05")
					}
				case nil:
					row[ei] = ""
				default:
					row[ei] = fmt.Sprintf("%v", v)
				}
			}

			// Extract row timestamp for 2-minute spacing
			if tsSourceIdx >= 0 {
				var rowTime time.Time
				val := values[tsSourceIdx]
				switch v := val.(type) {
				case time.Time:
					rowTime = v
				case []byte:
					rowTime, _ = time.Parse("2006-01-02 15:04:05", string(v))
				}
				if !prevTimestamp.IsZero() && !rowTime.IsZero() {
					diff := prevTimestamp.Sub(rowTime) // DESC order: prev is newer
					if diff < 2*time.Minute {
						continue
					}
				}
				if !rowTime.IsZero() {
					prevTimestamp = rowTime
				}
			}

			// Build a key from data columns only (exclude id/timestamp)
			dataVals := make([]string, len(dataColIndices))
			for j, idx := range dataColIndices {
				dataVals[j] = row[idx]
			}
			dataKey := strings.Join(dataVals, "|")

			// Skip if data is identical to previous row
			if dataKey == prevDataKey {
				continue
			}
			prevDataKey = dataKey

			writer.Write(row)
		}
		rows.Close()

		// Flush each batch immediately (frees memory)
		writer.Flush()

		// If we got fewer rows than batch size, we're done
		if rowCount < exportBatchSize {
			break
		}

		offset += exportBatchSize
	}

	// Send email with CSV attachment in background (don't block the response)
	csvCopy := make([]byte, emailBuf.Len())
	copy(csvCopy, emailBuf.Bytes())
	go sendCSVEmail(csvCopy, filename, table)
}

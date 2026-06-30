package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"grain_backend/database"
)

// machineLastState holds the last observed row identity for a machine table so
// consecutive /status calls can tell whether fresh data is actually arriving.
type machineLastState struct {
	ID          int
	Timestamp   time.Time
	LastChanged time.Time // wall-clock time when ID last changed
}

var (
	machineStateCache = make(map[string]machineLastState)
	machineStateMu    sync.Mutex
)

// MachineStatus represents the status of a machine
type MachineStatus struct {
	MachineStatus    bool   `json:"machineStatus"`
	CoolingStatus    bool   `json:"coolingStatus"`
	InternetStatus   bool   `json:"internetStatus"`
	MachineType      string `json:"machineType"`
	Priority         string `json:"priority"`
	ResponseType     string `json:"responseType"`
	NoNewData        bool   `json:"noNewData"`
	RecordID         int    `json:"recordId,omitempty"`
	LastUpdate       string `json:"lastUpdate"`
	HasNewData       bool   `json:"hasNewData"`
	IDChanged        bool   `json:"idChanged"`
	MachineName      string `json:"machineName"`
	TableName        string `json:"tableName"`
	CreatedAtChanged bool   `json:"createdAtChanged"`
}

// HandleMachineStatus handles machine status API
func HandleMachineStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	currentTime := time.Now()
	machines := []MachineStatus{}

	machineTables := map[string]string{
		"gtpl_122_s7_1200_01":       "GTPL_122",
		"kabomachinedatasmart200":   "KABO_200",
		"GTPL_108_gT_40E_P_S7_200_Germany": "GTPL_108",
		"GTPL_109_gT_40E_P_S7_200_Germany": "GTPL_109",
		"GTPL_110_gT_40E_P_S7_200_Germany": "GTPL_110",
		"GTPL_111_gT_80E_P_S7_200_Germany": "GTPL_111",
		"GTPL_112_gT_80E_P_S7_200_Germany": "GTPL_112",
		"GTPL_113_gT_80E_P_S7_200_Germany": "GTPL_113",
		"GTPL_114_GT_140E_S7_1200":  "GTPL_114",
		"GTPL_115_GT_180E_S7_1200":  "GTPL_115",
		"GTPL_119_GT_180E_S7_1200":  "GTPL_119",
		"GTPL_120_GT_180E_S7_1200":  "GTPL_120",
		"GTPL_116_GT_240E_S7_1200":  "GTPL_116",
		"GTPL_117_GT_320E_S7_1200":  "GTPL_117",
		"GTPL_121_GT1000T":          "GTPL_121",
		"GTPL_124_GT_450T_S7_1200":  "GTPL_124",
		"GTPL_081_GT_650T_S7_1200":  "GTPL_081",
		"GTPL_104_GT_650T_S7_1200":  "GTPL_104",
		"GTPL_105_GT_650T_S7_1200":  "GTPL_105",
		"GTPL_068_GT_650T_S7_1200":  "GTPL_068",
		"GTPL_133_GT_650T_S7_1200":  "GTPL_133",
		"GTPL_154_GT_650T_S7_1200":  "GTPL_154",
		"GTPL_155_GT_650T_S7_1200":  "GTPL_155",
		"GTPL_131_GT_650T_S7_1200":  "GTPL_131",
		"GTPL_132_GT300AP":          "GTPL_132",
		"GTPL_137_GT_450T_S7_1200":  "GTPL_137",
		"GTPL_138_GT_450T_S7_1200":  "GTPL_138",
		"GTPL_136_GT_450AP_S7_1200": "GTPL_136",
		"GTPL_134_GT_450T_S7_1200":  "GTPL_134",
		"GTPL_135_GT_450T_S7_1200":  "GTPL_135",
		"GTPL_061_GT_450T_S7_1200":  "GTPL_061",
		"GTPL_139_GT300AP":          "GTPL_139",
		"GTPL_142_GT_450AP_S7_1200": "GTPL_142",
		"GTPL_123_GT_450AP_S7_1200": "GTPL_123",
		"GTPL_143_GT_450AP_S7_1200": "GTPL_143",
		"GTPL_144_GT_300AP_S7_1200": "GTPL_144",
		"GTPL_145_GT_450T_S7_1200":  "GTPL_145",
		"GTPL_148_GT_450T_S7_1200":  "GTPL_148",
		"GTPL_118_GT_60T_S7_1200":   "GTPL_118",
	}

	for tableName, machineName := range machineTables {
		query := "SELECT * FROM `" + tableName + "` ORDER BY id DESC LIMIT 1"
		rows, err := database.SafeQuery(query)
		if err != nil {
			log.Printf("⚠️ Database error for table %s (%s): %v", tableName, machineName, err)
			// Still add machine to response, but mark as offline
			status := getMachineSpecificResponse(machineName, time.Time{}, currentTime, false)
			status.MachineName = machineName
			status.TableName = tableName
			machines = append(machines, status)
			continue
		}

		columns, _ := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if rows.Next() {
			rows.Scan(valuePtrs...)
			
			record := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					record[col] = string(b)
				} else if t, ok := val.(time.Time); ok {
					record[col] = t
				} else {
					record[col] = val
				}
			}

			// Extract ID and timestamp
			var id int
			var timestamp time.Time
			idFound := false

			if idVal, ok := record["id"]; ok {
				switch v := idVal.(type) {
				case int:
					id = v
					idFound = true
				case int64:
					id = int(v)
					idFound = true
				case int32:
					id = int(v)
					idFound = true
				case uint64:
					id = int(v)
					idFound = true
				case float64:
					id = int(v)
					idFound = true
				case string:
					// MySQL driver returns numeric columns as []byte, which the
					// row scan above converts to string. Parse it back.
					if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
						id = n
						idFound = true
					}
				case []byte:
					if n, err := strconv.Atoi(strings.TrimSpace(string(v))); err == nil {
						id = n
						idFound = true
					}
				}
			}

			// Check multiple possible timestamp column names
			for _, tsCol := range []string{"created_at", "created_on", "CreatedAt", "CreatedOn", "timestamp", "Timestamp", "DateTime", "datetime", "date_time", "Date", "date", "time"} {
				if tsVal, ok := record[tsCol]; ok {
					switch v := tsVal.(type) {
					case time.Time:
						if v.After(timestamp) {
							timestamp = v
						}
					case string:
						for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02T15:04:05Z07:00"} {
							if t, err := time.Parse(layout, v); err == nil {
								if t.After(timestamp) {
									timestamp = t
								}
								break
							}
						}
					}
				}
			}

			// Compare against the previously observed row for this table.
			// Check if this is actually new/fresh data by looking at:
			// 1. Has the ID changed? (new record inserted)
			// 2. Is the timestamp recent? (data still flowing)
			machineStateMu.Lock()
			prev, seen := machineStateCache[tableName]
			
			// ID changed means new data arrived
			idChanged := seen && idFound && id != prev.ID
			
			// If we haven't seen this machine before, don't consider it "new data"
			// On first observation, just cache it
			hasNewData := false
			if seen {
				// After first observation, data is "new" if:
				// 1. ID changed, OR
				// 2. Timestamp is more recent than 30 minutes ago (data is actively flowing)
				thirtyMinutesAgo := currentTime.Add(-30 * time.Minute)
				isRecentData := !timestamp.IsZero() && timestamp.After(thirtyMinutesAgo)
				hasNewData = idChanged || isRecentData
			}

			if idChanged {
				lastChanged := currentTime
				machineStateCache[tableName] = machineLastState{
					ID:          id,
					Timestamp:   timestamp,
					LastChanged: lastChanged,
				}
			} else if idFound {
				// Keep the existing LastChanged but update the timestamp
				machineStateCache[tableName] = machineLastState{
					ID:          id,
					Timestamp:   timestamp,
					LastChanged: prev.LastChanged,
				}
			}
			machineStateMu.Unlock()

			// Fresh data = ID changed OR timestamp is recent (within 1 minute)

			// Debug logging for GTPL_081 and GTPL_105
			if machineName == "GTPL_081" || machineName == "GTPL_105" {
				log.Printf("[DEBUG %s] ID: %d (prev: %d, changed: %v), hasNewData: %v", 
					machineName, id, prev.ID, idChanged, hasNewData)
			}

			status := getMachineSpecificResponse(machineName, timestamp, currentTime, hasNewData)
			status.RecordID = id
			status.LastUpdate = timestamp.Format(time.RFC3339)
			status.MachineName = machineName
			status.TableName = tableName
			status.HasNewData = hasNewData
			status.IDChanged = idChanged

			machines = append(machines, status)
		} else {
			// No rows at all for this machine — no id is coming, so it is
			// reported as offline rather than being silently omitted.
			status := getMachineSpecificResponse(machineName, time.Time{}, currentTime, false)
			status.MachineName = machineName
			status.TableName = tableName

			machines = append(machines, status)
		}
		rows.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "Machine status retrieved successfully",
		"data":      machines,
		"timestamp": currentTime.UTC().Format(time.RFC3339),
	})
}

func getMachineSpecificResponse(machineName string, timestamp, currentTime time.Time, hasNewData bool) MachineStatus {
	priority := "low"
	responseType := "unknown_machine"

	if machineName == "GTPL_122" {
		priority = "high"
		responseType = "gtpl_machine"
	} else if machineName == "KABO_200" {
		priority = "medium"
		responseType = "kabo_machine"
	} else if len(machineName) > 5 && machineName[:5] == "GTPL_" {
		priority = "medium"
		responseType = "gtpl_machine"
	}

	// A machine is considered online ONLY if it is actively updating (hasNewData = true).
	// If hasNewData is false, the machine is offline regardless of timestamp age.
	
	machineOnline := hasNewData
	coolingOnline := hasNewData
	internetOnline := hasNewData
	
	return MachineStatus{
		MachineStatus:  machineOnline,
		CoolingStatus:  coolingOnline,
		InternetStatus: internetOnline,
		MachineType:    machineName,
		Priority:       priority,
		ResponseType:   responseType,
		NoNewData:      !hasNewData,
	}
}

// HandleHealthCheck handles health check endpoint
func HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	dbStatus := "disconnected"
	if database.IsDatabaseConnected() {
		dbStatus = "connected"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"database":    dbStatus,
		"environment": "production",
	})
}

// HandleReports handles reports data retrieval
func HandleReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	table := r.URL.Query().Get("table")
	if table == "" {
		table = "kabomachinedatasmart200"
	}

	fromDate := r.URL.Query().Get("fromDate")
	toDate := r.URL.Query().Get("toDate")
	fromDate, toDate = normalizeDateRange(fromDate, toDate)
	pageStr := r.URL.Query().Get("page")
	
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 10
	hasDateFilter := fromDate != "" || toDate != ""

	allowedTables := getAllowedTables()
	if !contains(allowedTables, table) {
		http.Error(w, `{"error": "Invalid table name"}`, http.StatusBadRequest)
		return
	}

	// Detect timestamp column for this table
	tsCol := getTimestampColumn(table)

	// Expand date-only inputs ("2026-05-04") to full-day boundaries so a single
	// date matches the whole day instead of only midnight.
	fromDate, toDate = normalizeDateRange(fromDate, toDate)

	// Build WHERE clause. If the table has no timestamp column, ignore date filters
	// rather than producing an "Unknown column" SQL error.
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
		whereClause = "WHERE " + joinStrings(conditions, " AND ")
	}

	// Get total count
	countQuery := "SELECT COUNT(*) AS total FROM `" + table + "` " + whereClause
	countRows, err := database.SafeQuery(countQuery, params...)
	if err != nil {
		log.Printf("Reports count error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	defer countRows.Close()

	var total int
	if countRows.Next() {
		countRows.Scan(&total)
	}

	// Calculate offset
	offset := (page - 1) * limit

	// Get data
	dataParams := append(params, limit, offset)
	dataQuery := "SELECT * FROM `" + table + "` " + whereClause + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := database.SafeQuery(dataQuery, dataParams...)
	if err != nil {
		log.Printf("Reports data error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	data := scanRowsToMap(rows)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":            data,
		"page":            page,
		"limit":           limit,
		"total":           total,
		"table":           table,
		"timestampColumn": tsCol,
		"dateFilter": map[string]interface{}{
			"fromDate": fromDate,
			"toDate":   toDate,
			"applied":  hasDateFilter,
		},
	})
}

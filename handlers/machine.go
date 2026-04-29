package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"grain_backend/database"
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
		"GTPL_133_GT_650T_S7_1200":  "GTPL_133",
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
			continue // Skip tables that error
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
			
			if idVal, ok := record["id"]; ok {
				switch v := idVal.(type) {
				case int:
					id = v
				case int64:
					id = int(v)
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

			status := getMachineSpecificResponse(machineName, timestamp, currentTime, true)
			status.RecordID = id
			status.LastUpdate = timestamp.Format(time.RFC3339)
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
	fiveMinutesAgo := currentTime.Add(-5 * time.Minute)
	oneMinuteAgo := currentTime.Add(-1 * time.Minute)
	thirtySecondsAgo := currentTime.Add(-30 * time.Second)

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

	return MachineStatus{
		MachineStatus:  timestamp.After(fiveMinutesAgo),
		CoolingStatus:  timestamp.After(oneMinuteAgo),
		InternetStatus: timestamp.After(thirtySecondsAgo),
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

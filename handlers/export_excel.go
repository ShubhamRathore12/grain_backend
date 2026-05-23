package handlers

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"grain_backend/database"

	"github.com/xuri/excelize/v2"
)

// Columns to export for AP model machines (gT-450AP, gT-300AP)
// GTPL-123, 132, 136, 139, 142, 143, 144
var apModelColumns = []string{
	"id", "created_at", "created_on",
	"LP_value", "HP_value",
	"T2_temp_mean", "T1_temp_mean",
	"Blower_speed", "Condenser_fan_speed",
	"Hot_valve_speed", "AMV_valve_speed",
	"T1_set_point_in_grain_chilling_mode",
	"Delta_T_set_point_in_grain_chilling_mode",
	"T1_set_point_in_paddy_aging_mode",
	"Delta_T_set_point_paddy_aging_mode",
	"HP_set_point", "LP_set_point",
	"Grain_chilling_mode", "Paddy_aging_mode",
	"Manual_mode", "Aeration_mode",
	"CX_valve_25_percent_on_Q2_2",
	"CX_valve_50_percent_on_Q3_3",
	"CX_valve_75_percent_on_Q2_2",
	"CX_valve_100_percent_on_Q3_3",
	"FAULT_CODE",
}

// Columns to export for T model machines (gT-1000T, gT-650T, gT-450T)
// GTPL-121, 122, 133, 134, 135, 145
var tModelColumns = []string{
	"id", "created_at", "created_on",
	"LP_value", "HP_value",
	"T2_temp_mean", "T1_temp_mean", "T0_temp_mean",
	"Blower_speed", "Hot_valve_speed", "AHT_valve_speed",
	"T0_set_point", "Delta_T_set_point",
	"HP_set_point", "LP_set_point",
	"Auto_mode", "Manual_mode", "Aeration_mode",
	"CR_valve_25_percent_ON_Q0_2",
	"CR_valve_50_percent_ON_Q0_3",
	"CR_valve_75_percent_ON_Q2_2",
	"CR_valve_100_percent_ON_Q2_7",
	"FAULT_CODE",
}

// AP model machine prefixes
var apModelMachines = map[string]bool{
	"GTPL_123": true,
	"GTPL_132": true,
	"GTPL_136": true,
	"GTPL_139": true,
	"GTPL_142": true,
	"GTPL_143": true,
	"GTPL_144": true,
}

// T model machine prefixes
var tModelMachines = map[string]bool{
	"GTPL_081": true,
	"GTPL_105": true,
	"GTPL_121": true,
	"GTPL_122": true,
	"GTPL_133": true,
	"GTPL_134": true,
	"GTPL_135": true,
	"GTPL_145": true,
}

// Columns for GTPL-124 (gT-450T Indonesia) — same as T model but no CR_valve columns
var gtpl124Columns = []string{
	"id", "created_at", "created_on",
	"LP_value", "HP_value",
	"T2_temp_mean", "T1_temp_mean", "T0_temp_mean",
	"Blower_speed", "Hot_valve_speed", "AHT_valve_speed",
	"T0_set_point", "Delta_T_set_point",
	"HP_set_point", "LP_set_point",
	"Auto_mode", "Manual_mode", "Aeration_mode",
	"FAULT_CODE",
}

// isGTPL124 checks if table is GTPL-124
func isGTPL124(table string) bool {
	return getMachinePrefix(table) == "GTPL_124"
}

// Columns for GTPL-137, 138 (gT-450T Thailand) — like 124 but with Cond_fan_speed
var thailandTColumns = []string{
	"id", "created_at", "created_on",
	"LP_value", "HP_value",
	"T2_temp_mean", "T1_temp_mean", "T0_temp_mean",
	"Blower_speed", "Hot_valve_speed", "AHT_valve_speed", "Cond_fan_speed",
	"T0_set_point", "Delta_T_set_point",
	"HP_set_point", "LP_set_point",
	"Auto_mode", "Manual_mode", "Aeration_mode",
	"FAULT_CODE",
}

var thailandTMachines = map[string]bool{
	"GTPL_137": true,
	"GTPL_138": true,
}

func isThailandT(table string) bool {
	return thailandTMachines[getMachinePrefix(table)]
}

// Columns for GTPL-118 (gT-60T India) — has T1_set_point, Condenser_fan_speed, AHT_vale_speed
var gtpl118Columns = []string{
	"id", "created_at", "created_on",
	"LP_value", "HP_value",
	"T2_temp_mean", "T1_temp_mean", "T0_temp_mean",
	"Blower_speed", "Condenser_fan_speed", "Hot_valve_speed", "AHT_vale_speed",
	"T1_set_point", "Delta_T_set_point",
	"HP_set_point", "LP_set_point",
	"Auto_mode", "Manual_mode", "Aeration_mode",
	"FAULT_CODE",
}

func isGTPL118(table string) bool {
	return getMachinePrefix(table) == "GTPL_118"
}

// Columns for E model machines (gT-180E, gT-240E, gT-320E)
// GTPL-030, 115, 116, 117, 119, 120
var eModelColumns = []string{
	"id", "created_at", "created_on",
	"LP_value", "HP_value",
	"T2_temp_mean", "T1_temp_mean", "T0_temp_mean", "TH_temp_mean",
	"Blower_speed", "Hot_valve_speed", "AHT_vale_speed", "Heater_speed", "Cond_fan_speed",
	"T1_set_point", "TH_T1_set_point",
	"HP_set_point", "LP_set_point",
	"Auto_mode", "Manual_mode", "Aeration_mode",
}

var eModelMachines = map[string]bool{
	"GTPL_030": true,
	"GTPL_115": true,
	"GTPL_116": true,
	"GTPL_117": true,
	"GTPL_119": true,
	"GTPL_120": true,
}

func isEModel(table string) bool {
	return eModelMachines[getMachinePrefix(table)]
}

// Columns for EP model machines (gT-80EP, gT-40EP) — S7_200 Germany
// GTPL-108, 109, 110, 111, 112, 113
var epModelColumns = []string{
	"id", "created_at", "created_on",
	"AIR_OUTLET_TEMP", "AFTER_HEATER_TEMP_Th", "AIROUTLETTEMP",
	"AMBIENT_AIR_TEMP_T2", "COLD_AIR_TEMP_T1",
	"T1_SET_POINT", "Th_T1",
	"BLOWER_RPM", "AFTER_HEAT_VALVE_RPM", "HOT_GAS_VALVE_RPM", "CONDENSER_RPM",
	"DELTA_SET", "HP", "LP", "HP_SET_POINT",
	"AERATION_MODE_WITH_HEAT", "AERATION_MODE_WITHOUT_HEAT",
	"AUTO_EN", "MANUAL_EN",
	"Fault_Code1",
}

var epModelMachines = map[string]bool{
	"GTPL_108": true,
	"GTPL_109": true,
	"GTPL_110": true,
	"GTPL_111": true,
	"GTPL_112": true,
	"GTPL_113": true,
}

func isEPModel(table string) bool {
	return epModelMachines[getMachinePrefix(table)]
}

// getMachinePrefix extracts prefix like "GTPL_145" from table name
func getMachinePrefix(table string) string {
	parts := strings.Split(table, "_")
	if len(parts) >= 2 {
		prefix := parts[0] + "_" + parts[1]
		return strings.ToUpper(prefix)
	}
	return ""
}

// Indian machine prefixes - times should not be converted, keep as-is from database
var indianMachines = map[string]bool{
	"GTPL_081": true,
	"GTPL_105": true,
	"GTPL_118": true,
	"GTPL_121": true,
	"GTPL_122": true,
	"GTPL_123": true,
	"GTPL_132": true,
	"GTPL_133": true,
	"GTPL_134": true,
	"GTPL_135": true,
	"GTPL_139": true,
	"GTPL_142": true,
	"GTPL_143": true,
	"GTPL_144": true,
	"GTPL_145": true,
	"GTPL_148": true,
	"kabo": true,
}

// isIndianMachine checks if a table belongs to an Indian machine
func isIndianMachine(table string) bool {
	prefix := getMachinePrefix(table)
	if indianMachines[prefix] {
		return true
	}
	// Also check for kabo tables
	lower := strings.ToLower(table)
	return strings.HasPrefix(lower, "kabo")
}

// isAPModel checks if a table belongs to an AP model machine
func isAPModel(table string) bool {
	prefix := getMachinePrefix(table)
	return apModelMachines[prefix]
}

// isTModel checks if a table belongs to a T model machine
func isTModel(table string) bool {
	prefix := getMachinePrefix(table)
	return tModelMachines[prefix]
}

// promoteTimestampCols reorders columns so id, created_at, created_on appear first
// (in that order, only those present), preserving original order for the rest.
// Both the column names and their corresponding source indices are reordered together.
func promoteTimestampCols(cols []string, indices []int) ([]string, []int) {
	priority := []string{"id", "created_at", "created_on"}
	prioritySet := map[string]bool{"id": true, "created_at": true, "created_on": true}

	pos := map[string]int{}
	for i, c := range cols {
		pos[c] = i
	}

	outCols := []string{}
	outIdx := []int{}
	for _, p := range priority {
		if i, ok := pos[p]; ok {
			outCols = append(outCols, cols[i])
			outIdx = append(outIdx, indices[i])
		}
	}
	for i, c := range cols {
		if !prioritySet[c] {
			outCols = append(outCols, c)
			outIdx = append(outIdx, indices[i])
		}
	}
	return outCols, outIdx
}

// filterColumns returns only the columns that exist in the allowed list.
// id, created_at, created_on are always promoted to the front (in that order)
// so the export reads chronologically; remaining columns keep DB order.
func filterColumns(allColumns []string, allowed []string) ([]string, []int) {
	allowedSet := map[string]bool{}
	for _, c := range allowed {
		allowedSet[c] = true
	}

	priority := []string{"id", "created_at", "created_on"}
	prioritySet := map[string]bool{}
	for _, c := range priority {
		prioritySet[c] = true
	}

	colIndex := map[string]int{}
	for i, col := range allColumns {
		colIndex[col] = i
	}

	filtered := []string{}
	indices := []int{}

	// Front: id, created_at, created_on (only those that exist & are allowed).
	for _, col := range priority {
		if allowedSet[col] {
			if i, ok := colIndex[col]; ok {
				filtered = append(filtered, col)
				indices = append(indices, i)
			}
		}
	}

	// Rest: original DB order, skipping ones already added.
	for i, col := range allColumns {
		if allowedSet[col] && !prioritySet[col] {
			filtered = append(filtered, col)
			indices = append(indices, i)
		}
	}
	return filtered, indices
}

const excelBatchSize = 5000

// HandleExportExcel exports table data as an Excel (.xlsx) file
// Fetches in batches, skips duplicate rows, converts timestamps to machine timezone
// Filters columns for AP model machines
// Query params: table, fromDate, toDate, from, to, limit
func HandleExportExcel(w http.ResponseWriter, r *http.Request) {
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

	// Support both "from"/"to" and "fromDate"/"toDate"
	fromDate := r.URL.Query().Get("fromDate")
	if fromDate == "" {
		fromDate = r.URL.Query().Get("from")
	}
	toDate := r.URL.Query().Get("toDate")
	if toDate == "" {
		toDate = r.URL.Query().Get("to")
	}

	// Default to last 3 days
	if fromDate == "" {
		fromDate = time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	}
	if toDate == "" {
		toDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	}
	fromDate, toDate = normalizeDateRange(fromDate, toDate)

	// Optional row limit (default 50000)
	maxRows := 50000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			maxRows = l
		}
	}

	// Detect timestamp column and timezone
	tsCol := getTimestampColumn(table)
	machineTZ := getTimezoneForTable(table)

	// Get all columns from a single row
	colQuery := "SELECT * FROM `" + table + "` LIMIT 1"
	colRows, err := database.SafeQuery(colQuery)
	if err != nil {
		log.Printf("Excel export error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	allColumns, err := colRows.Columns()
	colRows.Close()
	if err != nil {
		log.Printf("Excel export columns error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}

	// Determine which columns to export
	exportColumns := allColumns
	exportIndices := make([]int, len(allColumns))
	for i := range allColumns {
		exportIndices[i] = i
	}

	// Filter columns based on machine model
	if isAPModel(table) {
		exportColumns, exportIndices = filterColumns(allColumns, apModelColumns)
	} else if isGTPL124(table) {
		exportColumns, exportIndices = filterColumns(allColumns, gtpl124Columns)
	} else if isThailandT(table) {
		exportColumns, exportIndices = filterColumns(allColumns, thailandTColumns)
	} else if isGTPL118(table) {
		exportColumns, exportIndices = filterColumns(allColumns, gtpl118Columns)
	} else if isEModel(table) {
		exportColumns, exportIndices = filterColumns(allColumns, eModelColumns)
	} else if isEPModel(table) {
		exportColumns, exportIndices = filterColumns(allColumns, epModelColumns)
	} else if isTModel(table) {
		exportColumns, exportIndices = filterColumns(allColumns, tModelColumns)
	}
	// Fallback if filtering returned nothing
	if len(exportColumns) == 0 {
		exportColumns = allColumns
		exportIndices = make([]int, len(allColumns))
		for i := range allColumns {
			exportIndices[i] = i
		}
	}

	// Always show id, created_at, created_on first.
	exportColumns, exportIndices = promoteTimestampCols(exportColumns, exportIndices)

	// Identify timestamp and dedup columns in export set
	tsColNames := map[string]bool{
		"created_at": true, "created_on": true, "CreatedAt": true, "CreatedOn": true,
		"updated_at": true, "updated_on": true, "UpdatedAt": true, "UpdatedOn": true,
		"timestamp": true, "Timestamp": true, "DateTime": true, "datetime": true,
		"date_time": true, "Date": true, "date": true, "time": true,
	}
	skipForDedup := map[string]bool{
		"id": true, "ID": true, "Id": true,
	}
	for k, v := range tsColNames {
		skipForDedup[k] = v
	}

	// Track which export columns are timestamps and which are for dedup
	tsExportIndices := map[int]bool{}
	dedupExportIndices := []int{}
	for i, col := range exportColumns {
		if tsColNames[col] {
			tsExportIndices[i] = true
		}
		if !skipForDedup[col] {
			dedupExportIndices = append(dedupExportIndices, i)
		}
	}

	// Create Excel file
	f := excelize.NewFile()
	sheetName := "Data"
	f.SetSheetName("Sheet1", sheetName)

	colCount := len(exportColumns)
	lastColName, _ := excelize.ColumnNumberToName(colCount)

	// ── Title row (Row 1): Company name ──
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 16, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#1A5276"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.MergeCell(sheetName, "A1", lastColName+"1")
	f.SetCellValue(sheetName, "A1", companyName+" - Remote Monitoring System")
	f.SetCellStyle(sheetName, "A1", lastColName+"1", titleStyle)
	f.SetRowHeight(sheetName, 1, 36)

	// ── Info row (Row 2): Machine, date range, generated at ──
	infoStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "#FFFFFF", Italic: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#2E86C1"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.MergeCell(sheetName, "A2", lastColName+"2")
	infoText := fmt.Sprintf("Machine: %s  |  Period: %s to %s  |  Generated: %s  |  %s  |  %s",
		table, fromDate, toDate, time.Now().Format("2006-01-02 15:04"), companyPhone, companyWebsite)
	f.SetCellValue(sheetName, "A2", infoText)
	f.SetCellStyle(sheetName, "A2", lastColName+"2", infoStyle)
	f.SetRowHeight(sheetName, 2, 22)

	// ── Empty spacer row (Row 3) ──
	f.SetRowHeight(sheetName, 3, 6)

	// ── Header row (Row 4) ──
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#1A5276"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "#0E3D5C", Style: 1},
			{Type: "top", Color: "#0E3D5C", Style: 1},
			{Type: "bottom", Color: "#0E3D5C", Style: 2},
			{Type: "right", Color: "#0E3D5C", Style: 1},
		},
	})
	for i, col := range exportColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		f.SetCellValue(sheetName, cell, col)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}
	f.SetRowHeight(sheetName, 4, 28)

	// ── Data row styles (alternating) ──
	dataStyleEven, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "#333333"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFFFFF"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#DEE2E6", Style: 1},
			{Type: "top", Color: "#DEE2E6", Style: 1},
			{Type: "bottom", Color: "#DEE2E6", Style: 1},
			{Type: "right", Color: "#DEE2E6", Style: 1},
		},
	})
	dataStyleOdd, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "#333333"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#EBF5FB"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#DEE2E6", Style: 1},
			{Type: "top", Color: "#DEE2E6", Style: 1},
			{Type: "bottom", Color: "#DEE2E6", Style: 1},
			{Type: "right", Color: "#DEE2E6", Style: 1},
		},
	})

	// Find the timestamp column index in source for 2-min spacing
	tsSourceIdx := -1
	for i, col := range allColumns {
		if col == tsCol {
			tsSourceIdx = i
			break
		}
	}

	// Fetch data in batches (data starts at row 5 after title/info/spacer/header)
	prevDataKey := ""
	var prevTimestamp time.Time
	rowNum := 5
	totalFetched := 0
	offset := 0

	for totalFetched < maxRows {
		batchLimit := excelBatchSize
		if totalFetched+batchLimit > maxRows {
			batchLimit = maxRows - totalFetched
		}

		var query string
		var queryArgs []interface{}
		if tsCol == "" {
			query = fmt.Sprintf("SELECT * FROM `%s` ORDER BY id DESC LIMIT %d OFFSET %d",
				table, batchLimit, offset)
		} else {
			query = fmt.Sprintf("SELECT * FROM `%s` WHERE `%s` >= ? AND `%s` <= ? ORDER BY id DESC LIMIT %d OFFSET %d",
				table, tsCol, tsCol, batchLimit, offset)
			queryArgs = []interface{}{fromDate, toDate}
		}

		rows, err := database.SafeQuery(query, queryArgs...)
		if err != nil {
			log.Printf("Excel export batch error at offset %d: %v", offset, err)
			break
		}

		batchCount := 0
		values := make([]interface{}, len(allColumns))
		valuePtrs := make([]interface{}, len(allColumns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		for rows.Next() {
			batchCount++
			rows.Scan(valuePtrs...)

			// Extract only the export columns
			// For Indian machines, keep times as-is from database (no timezone conversion)
			isIndian := isIndianMachine(table)
			exportVals := make([]string, len(exportIndices))
			for ei, ai := range exportIndices {
				val := values[ai]
				switch v := val.(type) {
				case []byte:
					if tsExportIndices[ei] {
						if t, err := time.Parse("2006-01-02 15:04:05", string(v)); err == nil {
							if isIndian {
								// Indian machines: keep time as-is from database
								exportVals[ei] = t.Format("2006-01-02 15:04:05")
							} else {
								// Other machines: convert to machine's local timezone
								exportVals[ei] = t.In(machineTZ).Format("2006-01-02 15:04:05")
							}
						} else {
							exportVals[ei] = string(v)
						}
					} else {
						exportVals[ei] = string(v)
					}
				case time.Time:
					if isIndian {
						// Indian machines: keep time as-is from database
						exportVals[ei] = v.Format("2006-01-02 15:04:05")
					} else {
						// Other machines: convert to machine's local timezone
						exportVals[ei] = v.In(machineTZ).Format("2006-01-02 15:04:05")
					}
				case nil:
					exportVals[ei] = ""
				default:
					exportVals[ei] = fmt.Sprintf("%v", v)
				}
			}

			// 2-minute spacing: skip rows less than 2 min from previous
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

			// Dedup check
			dedupVals := make([]string, len(dedupExportIndices))
			for j, idx := range dedupExportIndices {
				dedupVals[j] = exportVals[idx]
			}
			dataKey := strings.Join(dedupVals, "|")
			if dataKey == prevDataKey {
				continue
			}
			prevDataKey = dataKey

			// Write row to Excel with alternating row colors
			rowStyle := dataStyleEven
			if rowNum%2 == 1 {
				rowStyle = dataStyleOdd
			}
			for i, val := range exportVals {
				cell, _ := excelize.CoordinatesToCellName(i+1, rowNum)
				if num, err := strconv.ParseFloat(val, 64); err == nil && !tsExportIndices[i] {
					f.SetCellValue(sheetName, cell, num)
				} else {
					f.SetCellValue(sheetName, cell, val)
				}
				f.SetCellStyle(sheetName, cell, cell, rowStyle)
			}
			rowNum++
			totalFetched++
		}
		rows.Close()

		if batchCount < batchLimit {
			break
		}
		offset += batchLimit
	}

	// Auto-fit column widths
	for i, col := range exportColumns {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		width := float64(len(col)) + 4
		if width < 12 {
			width = 12
		}
		if width > 40 {
			width = 40
		}
		f.SetColWidth(sheetName, colName, colName, width)
	}

	// ── Footer row ──
	footerRow := rowNum + 1
	footerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: "#777777", Italic: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.MergeCell(sheetName, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("%s%d", lastColName, footerRow))
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", footerRow),
		fmt.Sprintf("%s | %s | %s | %s  -  Total Records: %d",
			companyName, companyWebsite, companyPhone, companyEmail, rowNum-5))
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("%s%d", lastColName, footerRow), footerStyle)

	// Freeze rows 1-4 (title + info + spacer + header)
	f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      4,
		TopLeftCell: "A5",
		ActivePane:  "bottomLeft",
	})

	// Auto-filter on header row
	lastCell := fmt.Sprintf("%s%d", lastColName, rowNum-1)
	f.AutoFilter(sheetName, fmt.Sprintf("A4:%s", lastCell), nil)

	// Set response headers
	dataRowCount := rowNum - 5
	filename := fmt.Sprintf("%s_%s_to_%s.xlsx", table, fromDate, toDate)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")

	// Write to response
	if err := f.Write(w); err != nil {
		log.Printf("Excel write error: %v", err)
	}

	// Also write to buffer for email
	var emailBuf bytes.Buffer
	f.Write(&emailBuf)
	f.Close()

	log.Printf("Excel export completed: %s, %d rows", filename, dataRowCount)

	// Send email with Excel attachment in background
	excelCopy := make([]byte, emailBuf.Len())
	copy(excelCopy, emailBuf.Bytes())
	go sendExcelEmail(excelCopy, filename, table, fromDate, toDate, dataRowCount)
}

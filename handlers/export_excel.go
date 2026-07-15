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

// getTemperatureColumns dynamically extracts columns for Excel export in order:
// 1. id, created_at, created_on
// 2. All temperature columns (T0, T1, T2, TH, etc.)
// 3. FAULT_CODE
// 4. All remaining columns from the table
func getTemperatureColumns(table string, allColumns []string) []string {
	// Start with standard timestamp columns
	result := []string{"id", "created_at", "created_on"}
	
	// Temperature patterns to look for
	tempPatterns := []string{
		"temp", "Temp", "TEMP",
		"T0", "T1", "T2", "TH", "Th",
		"AIR_OUTLET", "AMBIENT", "COLD_AIR", "AFTER_HEAT", "HEATER",
	}
	
	// Track what we've added
	added := make(map[string]bool)
	for _, col := range result {
		added[col] = true
	}
	
	// 1. Collect and add temperature columns
	tempCols := []string{}
	for _, col := range allColumns {
		if added[col] {
			continue
		}
		for _, pattern := range tempPatterns {
			if strings.Contains(col, pattern) {
				tempCols = append(tempCols, col)
				added[col] = true
				break
			}
		}
	}
	result = append(result, tempCols...)
	
	// 2. Add fault code columns
	faultCols := []string{"FAULT_CODE", "Fault_Code1"}
	for _, col := range faultCols {
		for _, tableCol := range allColumns {
			if tableCol == col && !added[col] {
				result = append(result, col)
				added[col] = true
				break
			}
		}
	}
	
	// 3. Add all remaining columns from the table
	for _, col := range allColumns {
		if !added[col] {
			result = append(result, col)
			added[col] = true
		}
	}
	
	return result
}

// filterColumnsForExcel returns only columns that exist in the table
func filterColumnsForExcel(allColumns []string, exportColumns []string) ([]string, []int) {
	colIndexMap := make(map[string]int)
	for i, col := range allColumns {
		colIndexMap[col] = i
	}
	
	filtered := []string{}
	indices := []int{}
	
	for _, col := range exportColumns {
		if idx, ok := colIndexMap[col]; ok {
			filtered = append(filtered, col)
			indices = append(indices, idx)
		}
	}
	
	return filtered, indices
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

// isIndianMachine checks if a table belongs to an Indian machine
func isIndianMachine(table string) bool {
	prefix := getMachinePrefix(table)
	indianMachines := map[string]bool{
		"GTPL_081": true, "GTPL_104": true, "GTPL_068": true, "GTPL_105": true,
		"GTPL_118": true, "GTPL_121": true, "GTPL_122": true, "GTPL_123": true,
		"GTPL_132": true, "GTPL_133": true, "GTPL_154": true, "GTPL_155": true,
		"GTPL_134": true, "GTPL_135": true, "GTPL_139": true, "GTPL_142": true,
		"GTPL_143": true, "GTPL_144": true, "GTPL_145": true, "GTPL_148": true,
		"kabo": true,
	}
	if indianMachines[prefix] {
		return true
	}
	return strings.HasPrefix(strings.ToLower(table), "kabo")
}

// Helper functions needed by export.go
// These are used for CSV export compatibility

var apModelColumns = []string{"id", "created_at"}
var tModelColumns = []string{"id", "created_at"}
var t650ModelColumns = []string{"id", "created_at"}
var gtpl124Columns = []string{"id", "created_at"}
var thailandTColumns = []string{"id", "created_at"}
var gtpl118Columns = []string{"id", "created_at"}
var eModelColumns = []string{"id", "created_at"}
var epModelColumns = []string{"id", "created_at"}

var apModelMachines = map[string]bool{}
var tModelMachines = map[string]bool{}
var t650ModelMachines = map[string]bool{}

func isAPModel(table string) bool { return false }
func isTModel(table string) bool { return false }
func isT650Model(table string) bool { return false }
func isGTPL124(table string) bool { return false }
func isThailandT(table string) bool { return false }
func isGTPL118(table string) bool { return false }
func isEModel(table string) bool { return false }
func isEPModel(table string) bool { return false }
func hasHeater(table string) bool { return false }
func dropHeaterColumns(cols []string, indices []int) ([]string, []int) { return cols, indices }

func filterColumns(allColumns []string, allowed []string) ([]string, []int) {
	return filterColumnsForExcel(allColumns, allowed)
}

func promoteTimestampCols(cols []string, indices []int) ([]string, []int) {
	return cols, indices
}

const excelBatchSize = 5000

// HandleExportExcel exports table data as Excel (.xlsx)
// Columns: id, created_at, all temperatures, FAULT_CODE, then all remaining columns
func HandleExportExcel(w http.ResponseWriter, r *http.Request) {
	log.Printf("[HandleExportExcel] START - endpoint called")

	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	table := r.URL.Query().Get("table")
	log.Printf("[HandleExportExcel] table param: %s", table)
	if table == "" {
		table = "kabomachinedatasmart200"
	}

	allowedTables := getAllowedTables()
	if !contains(allowedTables, table) {
		http.Error(w, `{"error": "Invalid table name"}`, http.StatusBadRequest)
		return
	}

	fromDate := r.URL.Query().Get("fromDate")
	if fromDate == "" {
		fromDate = r.URL.Query().Get("from")
	}
	toDate := r.URL.Query().Get("toDate")
	if toDate == "" {
		toDate = r.URL.Query().Get("to")
	}

	if fromDate == "" {
		fromDate = time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	}
	if toDate == "" {
		toDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	}
	fromDate, toDate = normalizeDateRange(fromDate, toDate)

	maxRows := 50000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			maxRows = l
		}
	}

	tsCol := getTimestampColumn(table)
	machineTZ := getTimezoneForTable(table)

	colQuery := "SELECT * FROM `" + table + "` LIMIT 1"
	colRows, err := database.SafeQuery(colQuery)
	if err != nil {
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	allColumns, err := colRows.Columns()
	colRows.Close()
	if err != nil {
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}

	exportColumns, exportIndices := applyColumnOrder(table, allColumns)

	log.Printf("[Excel] Table: %s, allColumns: %d, exportColumns: %d", table, len(allColumns), len(exportColumns))
	if len(exportColumns) == 0 {
		log.Printf("[Excel] ERROR: exportColumns empty! Reverting to database order")
		exportColumns = allColumns
		exportIndices = make([]int, len(allColumns))
		for i := range allColumns {
			exportIndices[i] = i
		}
	}
	if len(exportColumns) > 0 {
		log.Printf("[Excel] First 3 columns: %s, %s, %s", exportColumns[0], exportColumns[1], exportColumns[2])
	}

	tsColNames := map[string]bool{
		"created_at": true, "created_on": true, "CreatedAt": true, "CreatedOn": true,
		"updated_at": true, "updated_on": true, "UpdatedAt": true, "UpdatedOn": true,
		"timestamp": true, "Timestamp": true, "DateTime": true, "datetime": true,
		"date_time": true, "Date": true, "date": true, "time": true,
	}
	tsExportIndices := map[int]bool{}
	for i, col := range exportColumns {
		if tsColNames[col] {
			tsExportIndices[i] = true
		}
	}

	tsSourceIdx := -1
	for i, col := range allColumns {
		if col == tsCol {
			tsSourceIdx = i
			break
		}
	}

	f := excelize.NewFile()
	sheetName := "Data"
	f.SetSheetName("Sheet1", sheetName)

	colCount := len(exportColumns)
	lastColName, _ := excelize.ColumnNumberToName(colCount)

	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 16, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#1A5276"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.MergeCell(sheetName, "A1", lastColName+"1")
	f.SetCellValue(sheetName, "A1", companyName+" - Machine Data Report")
	f.SetCellStyle(sheetName, "A1", lastColName+"1", titleStyle)
	f.SetRowHeight(sheetName, 1, 36)

	infoStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "#FFFFFF", Italic: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#2E86C1"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.MergeCell(sheetName, "A2", lastColName+"2")
	infoText := fmt.Sprintf("Machine: %s | Period: %s to %s | Generated: %s", table, fromDate, toDate, time.Now().Format("2006-01-02 15:04"))
	f.SetCellValue(sheetName, "A2", infoText)
	f.SetCellStyle(sheetName, "A2", lastColName+"2", infoStyle)
	f.SetRowHeight(sheetName, 2, 22)

	f.SetRowHeight(sheetName, 3, 6)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#1A5276"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	for i, col := range exportColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		f.SetCellValue(sheetName, cell, col)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}
	f.SetRowHeight(sheetName, 4, 28)

	dataStyleEven, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "#333333"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFFFFF"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	dataStyleOdd, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "#333333"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#EBF5FB"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})

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
			query = fmt.Sprintf("SELECT * FROM `%s` ORDER BY id DESC LIMIT %d OFFSET %d", table, batchLimit, offset)
		} else {
			query = fmt.Sprintf("SELECT * FROM `%s` WHERE `%s` >= ? AND `%s` <= ? ORDER BY id DESC LIMIT %d OFFSET %d", table, tsCol, tsCol, batchLimit, offset)
			queryArgs = []interface{}{fromDate, toDate}
		}

		rows, err := database.SafeQuery(query, queryArgs...)
		if err != nil {
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

			allValuesMap := make(map[string]interface{})
			for i, col := range allColumns {
				allValuesMap[col] = values[i]
			}
			
			detectedFaultCode := detectFaultConditions(table, allValuesMap)

			exportVals := make([]string, len(exportIndices))
			for ei, ai := range exportIndices {
				val := values[ai]
				colName := exportColumns[ei]

				if colName == "FAULT_CODE" || colName == "Fault_Code1" {
					currentVal := ""
					switch v := val.(type) {
					case []byte:
						currentVal = string(v)
					case string:
						currentVal = v
					}

					seen := map[string]bool{}
					ordered := []string{}
					for _, f := range strings.Split(currentVal, ",") {
						f = strings.TrimSpace(f)
						if f != "" && !seen[f] {
							seen[f] = true
							ordered = append(ordered, f)
						}
					}
					for _, f := range strings.Split(detectedFaultCode, ",") {
						f = strings.TrimSpace(f)
						if f != "" && !seen[f] {
							seen[f] = true
							ordered = append(ordered, f)
						}
					}

					exportVals[ei] = strings.Join(ordered, ",")
					continue
				}

				switch v := val.(type) {
				case []byte:
					if tsExportIndices[ei] {
						if t, err := time.ParseInLocation("2006-01-02 15:04:05", string(v), sourceTZ); err == nil {
							exportVals[ei] = t.In(machineTZ).Format("2006-01-02 15:04:05")
						} else {
							exportVals[ei] = string(v)
						}
					} else {
						exportVals[ei] = string(v)
					}
				case time.Time:
					exportVals[ei] = v.In(machineTZ).Format("2006-01-02 15:04:05")
				case nil:
					exportVals[ei] = ""
				default:
					exportVals[ei] = fmt.Sprintf("%v", v)
				}
			}

			if tsSourceIdx >= 0 {
				var rowTime time.Time
				val := values[tsSourceIdx]
				switch v := val.(type) {
				case time.Time:
					rowTime = v
				case []byte:
					rowTime, _ = time.Parse("2006-01-02 15:04:05", string(v))
				}
				if !prevTimestamp.IsZero() && !rowTime.IsZero() && prevTimestamp.Sub(rowTime) < 2*time.Minute {
					continue
				}
				if !rowTime.IsZero() {
					prevTimestamp = rowTime
				}
			}

			dataKey := strings.Join(exportVals, "|")
			if dataKey == prevDataKey {
				continue
			}
			prevDataKey = dataKey

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

	footerRow := rowNum + 1
	footerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: "#777777", Italic: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.MergeCell(sheetName, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("%s%d", lastColName, footerRow))
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("Total Records: %d", rowNum-5))
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("%s%d", lastColName, footerRow), footerStyle)

	f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      4,
		TopLeftCell: "A5",
		ActivePane:  "bottomLeft",
	})

	lastCell := fmt.Sprintf("%s%d", lastColName, rowNum-1)
	f.AutoFilter(sheetName, fmt.Sprintf("A4:%s", lastCell), nil)

	dataRowCount := rowNum - 5
	filename := fmt.Sprintf("%s_%s_to_%s.xlsx", table, fromDate, toDate)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")

	if err := f.Write(w); err != nil {
		log.Printf("Excel write error: %v", err)
	}

	var emailBuf bytes.Buffer
	f.Write(&emailBuf)
	f.Close()

	log.Printf("Excel export completed: %s, %d rows", filename, dataRowCount)

	excelCopy := make([]byte, emailBuf.Len())
	copy(excelCopy, emailBuf.Bytes())
	go sendExcelEmail(excelCopy, filename, table, fromDate, toDate, dataRowCount)
}

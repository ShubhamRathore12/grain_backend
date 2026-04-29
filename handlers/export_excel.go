package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"grain_backend/database"

	"github.com/xuri/excelize/v2"
)

// HandleExportExcel exports table data as an Excel (.xlsx) file
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

	// Optional row limit (default 50000)
	maxRows := 50000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			maxRows = l
		}
	}

	// Query data
	query := "SELECT * FROM `" + table + "` WHERE created_at >= ? AND created_at <= ? ORDER BY id DESC LIMIT ?"
	rows, err := database.SafeQuery(query, fromDate, toDate, maxRows)
	if err != nil {
		log.Printf("Excel export error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Printf("Excel export columns error: %v", err)
		http.Error(w, `{"error": "Database error"}`, http.StatusInternalServerError)
		return
	}

	// Create Excel file
	f := excelize.NewFile()
	sheetName := "Data"
	f.SetSheetName("Sheet1", sheetName)

	// Style for header row
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#B0B0B0", Style: 1},
			{Type: "top", Color: "#B0B0B0", Style: 1},
			{Type: "bottom", Color: "#B0B0B0", Style: 1},
			{Type: "right", Color: "#B0B0B0", Style: 1},
		},
	})

	// Write header row
	for i, col := range columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, col)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// Write data rows
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	rowNum := 2
	for rows.Next() {
		rows.Scan(valuePtrs...)
		for i, val := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, rowNum)
			switch v := val.(type) {
			case []byte:
				// Try to parse as number
				str := string(v)
				if num, err := strconv.ParseFloat(str, 64); err == nil {
					f.SetCellValue(sheetName, cell, num)
				} else {
					f.SetCellValue(sheetName, cell, str)
				}
			case time.Time:
				f.SetCellValue(sheetName, cell, v.Format("2006-01-02 15:04:05"))
			case int64:
				f.SetCellValue(sheetName, cell, v)
			case float64:
				f.SetCellValue(sheetName, cell, v)
			case nil:
				f.SetCellValue(sheetName, cell, "")
			default:
				f.SetCellValue(sheetName, cell, fmt.Sprintf("%v", v))
			}
		}
		rowNum++
	}

	// Auto-fit column widths (approximate)
	for i, col := range columns {
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

	// Freeze header row
	f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	// Set response headers for Excel download
	filename := fmt.Sprintf("%s_%s_to_%s.xlsx", table, fromDate, toDate)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")

	// Write to response
	if err := f.Write(w); err != nil {
		log.Printf("Excel write error: %v", err)
	}
	f.Close()
}

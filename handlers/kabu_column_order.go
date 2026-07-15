package handlers

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	kabuColumnMap    = make(map[string][]string)
	kabuColumnsMutex = sync.Mutex{}
	columnsInitialized = false
)

// initializeKabuColumns reads and parses kabu_tables_column_order.md file once
func initializeKabuColumns() {
	kabuColumnsMutex.Lock()
	defer kabuColumnsMutex.Unlock()

	if columnsInitialized {
		return
	}

	// Try to find kabu_tables_column_order.md in project root
	mdPath := "kabu_tables_column_order.md"

	// If not found, try from executable directory
	if _, err := os.Stat(mdPath); err != nil {
		exePath, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exePath)
			mdPath = filepath.Join(exeDir, "kabu_tables_column_order.md")
		}
	}

	log.Printf("Reading kabu_tables_column_order.md from: %s", mdPath)
	content, err := os.ReadFile(mdPath)
	if err != nil {
		log.Printf("⚠ Could not read kabu_tables_column_order.md: %v", err)
		columnsInitialized = true
		return
	}

	parseKabuMarkdown(string(content))
	columnsInitialized = true
	log.Printf("✓ Loaded KABU table definitions for %d tables", len(kabuColumnMap))
}

// parseKabuMarkdown parses the markdown file and populates kabuColumnMap
func parseKabuMarkdown(content string) {
	tableNameRegex := regexp.MustCompile(`^##\s+\d+\.\s+([A-Za-z0-9_]+)`)
	lines := strings.Split(content, "\n")

	var pendingTables []string // tables waiting for column definition
	i := 0

	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Check if this is a table header
		matches := tableNameRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			tableName := matches[1]
			pendingTables = append(pendingTables, tableName)
			i++
			continue
		}

		// Skip empty lines and notes
		if line == "" || strings.HasPrefix(line, "*(") || strings.HasPrefix(line, "##") {
			i++
			continue
		}

		// If we have pending tables and this looks like a column line, parse it
		if len(pendingTables) > 0 && !strings.HasPrefix(line, "*") && strings.Contains(line, ",") {
			cols := parseColumnLine(line)
			if len(cols) > 0 {
				// Assign to all pending tables
				for _, tbl := range pendingTables {
					kabuColumnMap[tbl] = cols
					log.Printf("  → %s: %d columns", tbl, len(cols))
				}
				pendingTables = nil
			}
		}

		i++
	}
}


// parseColumnLine extracts column names from a comma-separated list
func parseColumnLine(line string) []string {
	parts := strings.Split(line, ",")
	result := make([]string, 0, len(parts))

	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" && !strings.HasPrefix(trimmed, "*") {
			result = append(result, trimmed)
		}
	}

	return result
}

// promoteFrontColumns moves created_at and created_on (in that order) to the
// front of the column list, keeping their source indices aligned. Used so every
// export (CSV + Excel) leads with the timestamp columns.
func promoteFrontColumns(cols []string, indices []int) ([]string, []int) {
	front := []string{"created_at", "created_on"}
	newCols := make([]string, 0, len(cols))
	newIdx := make([]int, 0, len(indices))
	used := make(map[int]bool)
	for _, name := range front {
		for i, c := range cols {
			if c == name && !used[i] {
				newCols = append(newCols, c)
				newIdx = append(newIdx, indices[i])
				used[i] = true
			}
		}
	}
	for i, c := range cols {
		if !used[i] {
			newCols = append(newCols, c)
			newIdx = append(newIdx, indices[i])
		}
	}
	return newCols, newIdx
}

// applyColumnOrder reorders database columns to match KABU specification
func applyColumnOrder(table string, allColumns []string) ([]string, []int) {
	initializeKabuColumns()

	log.Printf("applyColumnOrder called for table: '%s'", table)
	log.Printf("  kabuColumnMap has %d entries", len(kabuColumnMap))

	desiredOrder, ok := kabuColumnMap[table]
	if !ok {
		log.Printf("⚠ No KABU column order found for table: '%s'", table)
		// Check if table exists with different case
		for key := range kabuColumnMap {
			if strings.EqualFold(key, table) {
				log.Printf("  Found case-insensitive match: '%s'", key)
				desiredOrder = kabuColumnMap[key]
				ok = true
				break
			}
		}
		if !ok {
			log.Printf("  Using database order (no kabu mapping found)")
			indices := make([]int, len(allColumns))
			for i := range allColumns {
				indices[i] = i
			}
			return promoteFrontColumns(allColumns, indices)
		}
	}

	log.Printf("✓ Applying KABU column order for %s: %d desired cols, %d actual cols in DB", table, len(desiredOrder), len(allColumns))

	colIndexMap := make(map[string]int)
	for i, col := range allColumns {
		colIndexMap[col] = i
	}

	result := make([]string, 0, len(allColumns))
	resultIndices := make([]int, 0, len(allColumns))
	used := make(map[string]bool)

	// Count how many kabu columns exist in DB
	matchedCount := 0
	for _, col := range desiredOrder {
		if idx, ok := colIndexMap[col]; ok {
			result = append(result, col)
			resultIndices = append(resultIndices, idx)
			used[col] = true
			matchedCount++
		}
	}
	log.Printf("  → Matched %d of %d kabu columns from database", matchedCount, len(desiredOrder))

	// Append any columns from database that weren't in kabu spec
	for i, col := range allColumns {
		if !used[col] {
			result = append(result, col)
			resultIndices = append(resultIndices, i)
		}
	}

	return promoteFrontColumns(result, resultIndices)
}

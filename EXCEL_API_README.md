# Excel Download API Documentation

This API allows users to download data from various database tables in Excel format.

## API Endpoints

### 1. Get Available Tables

**GET** `/api/excel/tables`

Returns a list of all available tables that can be exported to Excel.

**Response:**

```json
{
  "success": true,
  "tables": [
    "GTPL_108_gT_40E_P_S7_200_Germany",
    "GTPL_109_gT_40E_P_S7_200_Germany",
    "kabomachinedatasmart200"
    // ... more tables
  ]
}
```

### 2. Download Excel File

**GET** `/api/excel/download-excel`

Downloads data from a specified table as an Excel file.

**Query Parameters:**

- `table` (required): The name of the table to export
- `limit` (optional): Number of rows to export (default: 1000, max: 10000)
- `offset` (optional): Number of rows to skip for pagination (default: 0)
- `startDate` (optional): Filter data from this date (format: YYYY-MM-DD)
- `endDate` (optional): Filter data until this date (format: YYYY-MM-DD)

**Example Request:**

```
GET /api/excel/download-excel?table=kabomachinedatasmart200&limit=500&offset=0&startDate=2024-01-01&endDate=2024-01-31
```

**Response:**

- Success: Excel file download
- Error: JSON error message

## Available Tables

The following tables are available for Excel export:

1. `GTPL_108_gT_40E_P_S7_200_Germany`
2. `GTPL_109_gT_40E_P_S7_200_Germany`
3. `GTPL_110_gT_40E_P_S7_200_Germany`
4. `GTPL_111_gT_80E_P_S7_200_Germany`
5. `GTPL_112_gT_80E_P_S7_200_Germany`
6. `GTPL_113_gT_80E_P_S7_200_Germany`
7. `kabomachinedatasmart200`
8. `GTPL_114_GT_140E_S7_1200`
9. `GTPL_115_GT_180E_S7_1200`
10. `GTPL_119_GT_180E_S7_1200`
11. `GTPL_120_GT_180E_S7_1200`
12. `GTPL_116_GT_240E_S7_1200`
13. `GTPL_117_GT_320E_S7_1200`
14. `GTPL_121_GT1000T`
15. `gtpl_122_s7_1200_01`

## Usage Examples

### Using JavaScript/Fetch

```javascript
// Get available tables
const response = await fetch("/api/excel/tables");
const data = await response.json();
console.log(data.tables);

// Download Excel file
const downloadResponse = await fetch(
  "/api/excel/download-excel?table=kabomachinedatasmart200&limit=1000"
);
if (downloadResponse.ok) {
  const blob = await downloadResponse.blob();
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "data.xlsx";
  a.click();
  window.URL.revokeObjectURL(url);
}
```

### Using cURL

```bash
# Get available tables
curl -X GET http://localhost:3000/api/excel/tables

# Download Excel file
curl -X GET "http://localhost:3000/api/excel/download-excel?table=kabomachinedatasmart200&limit=500" \
  -H "Accept: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" \
  --output data.xlsx
```

## React Component

A React component (`ExcelDownloadComponent.jsx`) is provided in the `react/` directory that provides a user-friendly interface for downloading Excel files.

**Features:**

- Dropdown to select table
- Input fields for limit and offset
- Date range filters
- Loading states and error handling
- Automatic file download

## Error Handling

The API returns appropriate HTTP status codes and error messages:

- `400 Bad Request`: Invalid parameters (table name, limit, offset)
- `404 Not Found`: No data found for the specified criteria
- `500 Internal Server Error`: Server-side error

**Error Response Format:**

```json
{
  "success": false,
  "error": "Error message description",
  "message": "Detailed error message"
}
```

## Security Considerations

- Only predefined tables are allowed (whitelist approach)
- Limit is capped at 10,000 rows to prevent memory issues
- Input validation for all parameters
- SQL injection protection through parameterized queries

## Dependencies

The Excel functionality requires the following npm package:

- `exceljs`: For generating Excel files

Install with:

```bash
npm install exceljs
```

## File Format

The generated Excel files:

- Use `.xlsx` format (Excel 2007+)
- Include styled headers (bold, gray background)
- Auto-fit column widths
- Include timestamp in filename
- Contain all columns from the database table



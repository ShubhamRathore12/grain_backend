const express = require("express");
const router = express.Router();
const { pool } = require("../db");
const XLSX = require("xlsx");
const { URL } = require("url");

const ALLOWED_TABLES = [
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
];

// More flexible limits - allow larger downloads but with warnings
const MAX_RECORDS_DOWNLOAD = 50000; // Increased limit
const QUERY_TIMEOUT = 10000; // 10 seconds max per query

function withTimeout(promise, ms, label) {
  return Promise.race([
    promise,
    new Promise((_, reject) =>
      setTimeout(() => reject(new Error(`${label} timeout`)), ms)
    ),
  ]);
}

// Unified endpoint: if downloadAll=true returns XLSX, otherwise returns paginated JSON
router.get("/download-excel", async (req, res) => {
  const startTime = Date.now();
  try {
    const table = (req.query.table || "kabomachinedatasmart200").toString();
    const page = parseInt((req.query.page || "1").toString(), 10);
    const fromDate = req.query.fromDate
      ? req.query.fromDate.toString()
      : undefined;
    const toDate = req.query.toDate ? req.query.toDate.toString() : undefined;
    const downloadAll =
      (req.query.downloadAll || "false").toString() === "true";
    const forceLargeDownload =
      (req.query.forceLarge || "false").toString() === "true";

    // Validate table name
    if (!ALLOWED_TABLES.includes(table)) {
      return res.status(400).json({
        error: "Invalid table name",
        allowedTables: ALLOWED_TABLES,
      });
    }

    // Assume created_at exists as timestamp column
    const timestampColumn = "created_at";

    // Build base query
    let baseQuery = `FROM \`${table}\``;
    const queryParams = [];
    const conditions = [];

    if (fromDate) {
      conditions.push(`\`${timestampColumn}\` >= ?`);
      queryParams.push(fromDate);
    }
    if (toDate) {
      conditions.push(`\`${timestampColumn}\` <= ?`);
      queryParams.push(toDate);
    }
    if (conditions.length > 0) {
      baseQuery += ` WHERE ${conditions.join(" AND ")}`;
    }

    if (downloadAll) {
      try {
        // Count total
        const [countRows] = await withTimeout(
          pool.query(`SELECT COUNT(*) as total ${baseQuery}`, queryParams),
          QUERY_TIMEOUT,
          "Count query"
        );
        const total = Number(countRows?.[0]?.total || 0);

        if (total === 0) {
          return res.status(404).json({
            error: "No data found",
            table,
            dateFilter: { fromDate: fromDate || null, toDate: toDate || null },
          });
        }

        if (total > MAX_RECORDS_DOWNLOAD && !forceLargeDownload) {
          const url = new URL(
            req.protocol + "://" + req.get("host") + req.originalUrl
          );
          url.searchParams.set("forceLarge", "true");
          return res.status(400).json({
            error: `Large dataset detected (${total} records)`,
            suggestion:
              "Consider using date filters to reduce data size for better performance",
            maxAllowed: MAX_RECORDS_DOWNLOAD,
            downloadUrl: url.toString(),
            warning:
              "Large downloads may take several minutes and consume significant memory",
            estimatedTime: `${Math.ceil(total / 1000)} seconds`,
          });
        }

        // Data query
        const limitForQuery = Math.min(total, MAX_RECORDS_DOWNLOAD);
        const dataQuery = `SELECT * ${baseQuery} ORDER BY id DESC LIMIT ${limitForQuery}`;

        const [dataRows] = await withTimeout(
          pool.query(dataQuery, queryParams),
          Math.max(QUERY_TIMEOUT, total > 5000 ? 30000 : 10000),
          "Data query"
        );

        const rows = Array.isArray(dataRows) ? dataRows : [];
        if (rows.length === 0) {
          return res.status(404).json({ error: "No data records found" });
        }

        // Process data
        const processedData = rows.map((row) => {
          const flattened = {};
          for (const key of Object.keys(row)) {
            const value = row[key];
            if (value === null || value === undefined) {
              flattened[key] = "";
            } else if (value instanceof Date) {
              flattened[key] = value
                .toISOString()
                .slice(0, 19)
                .replace("T", " ");
            } else {
              flattened[key] = String(value);
            }
          }
          return flattened;
        });

        // Build XLSX
        const workbook = XLSX.utils.book_new();
        const worksheet = XLSX.utils.json_to_sheet(processedData);
        XLSX.utils.book_append_sheet(workbook, worksheet, "Data");
        const excelBuffer = XLSX.write(workbook, {
          bookType: "xlsx",
          type: "buffer",
        });

        const timestamp = new Date().toISOString().slice(0, 10);
        const filename = `${table}_${timestamp}.xlsx`;

        res.setHeader(
          "Content-Type",
          "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        );
        res.setHeader(
          "Content-Disposition",
          `attachment; filename="${filename}"`
        );
        res.setHeader("Content-Length", String(excelBuffer.length));
        res.setHeader("Cache-Control", "no-cache, no-store, must-revalidate");
        return res.status(200).send(excelBuffer);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        if (message.includes("timeout")) {
          return res.status(408).json({
            error: "Request timeout - too much data to process",
            suggestion:
              "Please try with a smaller date range or contact support",
            processingTime: `${Date.now() - startTime}ms`,
          });
        }
        return res.status(500).json({
          error: "Download failed",
          details: message,
          processingTime: `${Date.now() - startTime}ms`,
        });
      }
    }

    // Paginated JSON response (when downloadAll is false)
    const limit = 100;
    const offset = (page - 1) * limit;

    try {
      const [countPair, dataPair] = await Promise.all([
        withTimeout(
          pool.query(`SELECT COUNT(*) as total ${baseQuery}`, queryParams),
          QUERY_TIMEOUT,
          "Count timeout"
        ),
        withTimeout(
          pool.query(
            `SELECT * ${baseQuery} ORDER BY id DESC LIMIT ${limit} OFFSET ${offset}`,
            queryParams
          ),
          QUERY_TIMEOUT,
          "Data timeout"
        ),
      ]);

      const [countRows] = countPair; // rows for count
      const [rows] = dataPair; // rows for data

      const total = Number(countRows?.[0]?.total || 0);

      return res.json({
        table,
        data: rows,
        total,
        page,
        limit,
        totalPages: Math.ceil(total / limit),
        timestampColumn: timestampColumn || null,
        dateFilter: {
          fromDate: fromDate || null,
          toDate: toDate || null,
          applied: Boolean(timestampColumn && (fromDate || toDate)),
        },
        processingTime: `${Date.now() - startTime}ms`,
      });
    } catch (queryError) {
      const message =
        queryError instanceof Error ? queryError.message : String(queryError);
      return res.status(500).json({
        error: "Database query failed",
        details: message,
        processingTime: `${Date.now() - startTime}ms`,
      });
    }
  } catch (err) {
    return res.status(500).json({
      error: "Internal server error",
      message: err?.message || "Unknown error",
      processingTime: `${Date.now() - startTime}ms`,
      timestamp: new Date().toISOString(),
    });
  }
});

// GET endpoint to get available tables
router.get("/tables", (req, res) => {
  res.json({
    success: true,
    tables: ALLOWED_TABLES,
  });
});

module.exports = router;

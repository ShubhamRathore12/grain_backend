const { pool } = require("../db");
const XLSX = require("xlsx");
const { Readable } = require("stream");

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
  "GTPL_124_GT_450T_S7_1200",
  "GTPL_131_GT_650T_S7_1200",
  "GTPL_132_GT_650T_S7_1200",
  "GTPL_132_GT300AP",
  "GTPL_137_GT_450T_S7_1200",
  "GTPL_138_GT_450T_S7_1200",
  "GTPL_061_GT_450T_S7_1200",
  "GTPL_134_GT_450T_S7_1200",
  "GTPL_135_GT_450T_S7_1200",
  "GTPL_139_GT300AP"
];

const express = require("express");
const router = express.Router();

router.get("/", async (req, res) => {
  try {
    const table = req.query.table || "kabomachinedatasmart200";
    const fromDate = req.query.fromDate;
    const toDate = req.query.toDate;

    if (!ALLOWED_TABLES.includes(table)) {
      return res.status(400).json({
        error: "Invalid table name",
        allowedTables: ALLOWED_TABLES,
      });
    }

    const hasDateFilter = !!(fromDate || toDate);

    // If NO date filter -> hardcode limit to 100, allow ?page (defaults to 1)
    const page = Math.max(1, parseInt(req.query.page || "1", 10));
    const limit = hasDateFilter ? undefined : 10; // hardcoded 100 when no dates
    const offset = hasDateFilter ? undefined : (page - 1) * limit;

    // Build WHERE
    const whereClauses = [];
    const params = [];
    if (fromDate) {
      whereClauses.push("created_at >= ?");
      params.push(fromDate);
    }
    if (toDate) {
      whereClauses.push("created_at <= ?");
      params.push(toDate);
    }
    const whereSql = whereClauses.length ? `WHERE ${whereClauses.join(" AND ")}` : "";

    // Total count (for pagination UI)
    // - When date filter: count the filtered set
    // - When no date filter: count entire table
    const countSql = `SELECT COUNT(*) AS total FROM \`${table}\` ${whereSql}`;
    const [countRows] = await pool.query(countSql, params);
    const total = Array.isArray(countRows) && countRows[0]?.total ? Number(countRows[0].total) : 0;

    // Data query
    let dataSql = `SELECT * FROM \`${table}\` ${whereSql} ORDER BY id DESC`;
    const dataParams = [...params];

    if (!hasDateFilter) {
      // Hardcode 100 rows when no date range
      dataSql += ` LIMIT ? OFFSET ?`;
      dataParams.push(limit, offset);
    }
    const [rows] = await pool.query(dataSql, dataParams);

    // Normalize rows: Format dates exactly as stored, no timezone conversion, no "T"
    const data = (Array.isArray(rows) ? rows : []).map((row) => {
      const obj = {};
      for (const [k, v] of Object.entries(row)) {
        if (v === null || v === undefined) obj[k] = "";
        else if (v instanceof Date) {
          // Format as YYYY-MM-DD HH:mm:ss exactly without adding "T" or timezone shifts
          const year = v.getFullYear();
          const month = String(v.getMonth() + 1).padStart(2, "0");
          const day = String(v.getDate()).padStart(2, "0");
          const hours = String(v.getHours()).padStart(2, "0");
          const minutes = String(v.getMinutes()).padStart(2, "0");
          const seconds = String(v.getSeconds()).padStart(2, "0");
          obj[k] = `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
        } else {
          obj[k] = v;
        }
      }
      return obj;
    });

    return res.json({
      data,
      page: hasDateFilter ? 1 : page,
      limit: hasDateFilter ? total : limit,
      total,
      table,
      timestampColumn: "created_at",
      dateFilter: {
        fromDate: fromDate || undefined,
        toDate: toDate || undefined,
        applied: hasDateFilter,
      },
    });
  } catch (err) {
    console.error("DB fetch error:", err?.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

// Add download functionality
router.get("/download", async (req, res) => {
  try {
    const table = req.query.table || "kabomachinedatasmart200";
    const fromDate = req.query.fromDate;
    const toDate = req.query.toDate;

    if (!ALLOWED_TABLES.includes(table)) {
      return res.status(400).json({
        error: "Invalid table name",
        allowedTables: ALLOWED_TABLES,
      });
    }

    // Build WHERE clause for download
    const whereClauses = [];
    const params = [];
    if (fromDate) {
      whereClauses.push("created_at >= ?");
      params.push(fromDate);
    }
    if (toDate) {
      whereClauses.push("created_at <= ?");
      params.push(toDate);
    }
    const whereSql = whereClauses.length ? `WHERE ${whereClauses.join(" AND ")}` : "";

    // Get all data for download
    const dataSql = `SELECT * FROM \`${table}\` ${whereSql} ORDER BY id DESC`;
    const [rows] = await pool.query(dataSql, params);

    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: "No data found for the selected date range" });
    }

    // Normalize data for Excel export
    const exportData = rows.map((row) => {
      const obj = {};
      for (const [k, v] of Object.entries(row)) {
        if (v === null || v === undefined) obj[k] = "";
        else if (v instanceof Date) {
          const year = v.getFullYear();
          const month = String(v.getMonth() + 1).padStart(2, "0");
          const day = String(v.getDate()).padStart(2, "0");
          const hours = String(v.getHours()).padStart(2, "0");
          const minutes = String(v.getMinutes()).padStart(2, "0");
          const seconds = String(v.getSeconds()).padStart(2, "0");
          obj[k] = `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
        } else {
          obj[k] = v;
        }
      }
      return obj;
    });

    // Create Excel file
    const workbook = XLSX.utils.book_new();
    const worksheet = XLSX.utils.json_to_sheet(exportData);
    XLSX.utils.book_append_sheet(workbook, worksheet, "Data");
    const excelBuffer = XLSX.write(workbook, {
      bookType: "xlsx",
      type: "buffer",
    });

    res.set({
      "Content-Type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      "Content-Disposition": `attachment; filename="${table}_export_${fromDate || "start"}_to_${toDate || "end"}.xlsx"`,
      "Content-Length": excelBuffer.length,
    });

    const chunkSize = 1024 * 1024;
    const readable = new Readable({
      read() {
        let position = 0;
        const pushChunk = () => {
          if (position >= excelBuffer.length) {
            this.push(null);
            return;
          }
          const chunk = excelBuffer.slice(position, position + chunkSize);
          this.push(chunk);
          position += chunkSize;
          const progress = Math.round((position / excelBuffer.length) * 100);
          console.log(`Export progress: ${progress}%`);
          setImmediate(pushChunk);
        };
        pushChunk();
      },
    });

    return readable.pipe(res);
  } catch (err) {
    console.error("Download error:", err?.message || err);
    res.status(500).json({ error: "Download error" });
  }
});

module.exports = router;

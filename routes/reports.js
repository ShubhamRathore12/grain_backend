const express = require("express");
const router = express.Router();
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
];

router.get("/", async (req, res) => {
  try {
    const table = req.query.table || "kabomachinedatasmart200";
    const page = parseInt(req.query.page || "1", 10);
    const fromDate = req.query.fromDate;
    const toDate = req.query.toDate;
    const downloadAll = req.query.downloadAll === "true";
    const limit = downloadAll ? 10000 : 100;
    const offset = (page - 1) * limit;

    if (!ALLOWED_TABLES.includes(table)) {
      return res.status(400).json({
        error: "Invalid table name",
        allowedTables: ALLOWED_TABLES,
      });
    }

    let timestampColumn = "";
    try {
      const [checkLower] = await pool.query(
        `SHOW COLUMNS FROM \`${table}\` LIKE 'created_at'`
      );
      if (checkLower.length > 0) {
        timestampColumn = "created_at";
      } else {
        const [checkUpper] = await pool.query(
          `SHOW COLUMNS FROM \`${table}\` LIKE 'created_At'`
        );
        if (checkUpper.length > 0) {
          timestampColumn = "created_At";
        }
      }
    } catch (err) {
      console.error("Error checking timestamp columns:", err);
    }

    let countQuery = `SELECT COUNT(*) as total FROM \`${table}\``;
    let dataQuery = `SELECT * FROM \`${table}\``;
    const queryParams = [];

    if (timestampColumn && (fromDate || toDate)) {
      let dateCondition = "";
      if (fromDate) {
        dateCondition += `\`${timestampColumn}\` >= ?`;
        queryParams.push(fromDate);
      }
      if (toDate) {
        if (fromDate) dateCondition += " AND ";
        dateCondition += `\`${timestampColumn}\` <= ?`;
        queryParams.push(toDate);
      }
      if (dateCondition) {
        countQuery += ` WHERE ${dateCondition}`;
        dataQuery += ` WHERE ${dateCondition}`;
      }
    }

    if (downloadAll) {
      const [countResult] = await pool.query(countQuery, queryParams);
      const total = countResult[0]?.total || 0;
      if (total === 0) {
        return res
          .status(404)
          .json({ error: "No data found for the selected date range" });
      }
      const [rows] = await pool.query(
        `${dataQuery} ORDER BY id DESC`,
        queryParams
      );
      const workbook = XLSX.utils.book_new();
      const worksheet = XLSX.utils.json_to_sheet(rows);
      XLSX.utils.book_append_sheet(workbook, worksheet, "Data");
      const excelBuffer = XLSX.write(workbook, {
        bookType: "xlsx",
        type: "buffer",
      });

      res.set({
        "Content-Type":
          "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        "Content-Disposition": `attachment; filename="${table}_export_${
          fromDate || "start"
        }_to_${toDate || "end"}.xlsx"`,
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
    }

    dataQuery += ` ORDER BY id DESC LIMIT ? OFFSET ?`;
    queryParams.push(limit, offset);

    const [countResult] = await pool.query(countQuery, queryParams.slice(0, -2));
    const total = countResult[0]?.total || 0;
    const [rows] = await pool.query(dataQuery, queryParams);

    return res.json({
      table,
      data: rows,
      total,
      page,
      limit,
      totalPages: Math.ceil(total / limit),
      timestampColumn: timestampColumn || null,
      dateFilter: {
        fromDate,
        toDate,
        applied: Boolean(timestampColumn && (fromDate || toDate)),
      },
    });
  } catch (err) {
    console.error("DB fetch error:", err?.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

module.exports = router;

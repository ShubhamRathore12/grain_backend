const express = require("express");
const router = express.Router();
const { pool } = require("../db");

const allowedTables = new Set([
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
  "gtpl_122_s7_1200_01"
]);

router.get("/", async (req, res) => {
  try {
    const table = req.query.table;
    if (!table || !allowedTables.has(table)) {
      return res.status(400).json({ error: "Invalid or missing table name" });
    }
    const [rows] = await pool.query(`SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 1`);
    res.status(200).json({ table, data: rows?.[0] || null });
  } catch (err) {
    console.error("DB fetch error:", err?.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

module.exports = router;
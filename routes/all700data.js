const express = require("express");
const router = express.Router();
const { pool } = require("../db"); // Adjust path if needed
const { MACHINE_CONFIG } = require("../utils/machineConfig");

const allowedTables = new Set(
  Object.values(MACHINE_CONFIG).map((c) => c.table)
);

// GET latest 100 rows - accepts ?table= param
router.get("/getAllDataSmart200", async (req, res) => {
  try {
    const table = req.query.table || "kabomachinedatasmart200";

    if (!allowedTables.has(table)) {
      return res.status(400).json({ error: "Invalid table name" });
    }

    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 100`
    );
    res.status(200).json(rows);
  } catch (err) {
    console.error("DB fetch error:", err.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

// GET latest 100 rows - accepts ?table= param
router.get("/getAllData", async (req, res) => {
  try {
    const table = req.query.table || "gtpl_122_s7_1200_01";

    if (!allowedTables.has(table)) {
      return res.status(400).json({ error: "Invalid table name" });
    }

    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 100`
    );
    res.status(200).json(rows);
  } catch (err) {
    console.error("DB fetch error:", err.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

// GET paginated rows - accepts ?table= param
router.get("/paginatedSmart200", async (req, res) => {
  let page = parseInt(req.query.page, 10) || 1;
  let limit = parseInt(req.query.limit, 10) || 10;
  const table = req.query.table || "kabomachinedatasmart200";
  const from = req.query.from; // expected format: 'YYYY-MM-DD'
  const to = req.query.to;     // expected format: 'YYYY-MM-DD'

  if (!allowedTables.has(table)) {
    return res.status(400).json({ error: "Invalid table name" });
  }

  if (page < 1) page = 1;
  if (limit < 1) limit = 10;
  const offset = (page - 1) * limit;

  let filters = "";
  const params = [];

  // Handle date filtering conditionally
  if (from && to) {
    filters = "WHERE created_at BETWEEN ? AND ?";
    params.push(from, to);
  } else if (from) {
    filters = "WHERE created_at >= ?";
    params.push(from);
  } else if (to) {
    filters = "WHERE created_at <= ?";
    params.push(to);
  }

  try {
    // Get total filtered count
    const [countRows] = await pool.query(
      `SELECT COUNT(*) as total FROM \`${table}\` ${filters}`,
      params
    );
    const total = countRows[0]?.total || 0;

    // Get filtered + paginated data
    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` ${filters} ORDER BY id DESC LIMIT ? OFFSET ?`,
      [...params, limit, offset]
    );

    res.status(200).json({
      data: rows,
      page,
      limit,
      total,
      totalPages: Math.ceil(total / limit),
    });
  } catch (err) {
    console.error("DB fetch error:", err.message || err);
    res.status(500).json({ error: "Database error" });
  }
});


router.get("/paginatedSmart1200", async (req, res) => {
  let page = parseInt(req.query.page, 10) || 1;
  let limit = parseInt(req.query.limit, 10) || 10;
  const table = req.query.table || "gtpl_122_s7_1200_01";
  const from = req.query.from; // expected format: 'YYYY-MM-DD'
  const to = req.query.to;     // expected format: 'YYYY-MM-DD'

  if (!allowedTables.has(table)) {
    return res.status(400).json({ error: "Invalid table name" });
  }

  if (page < 1) page = 1;
  if (limit < 1) limit = 10;
  const offset = (page - 1) * limit;

  let filters = "";
  const params = [];

  // Handle date filtering conditionally
  if (from && to) {
    filters = "WHERE created_at BETWEEN ? AND ?";
    params.push(from, to);
  } else if (from) {
    filters = "WHERE created_at >= ?";
    params.push(from);
  } else if (to) {
    filters = "WHERE created_at <= ?";
    params.push(to);
  }

  try {
    // Get total filtered count
    const [countRows] = await pool.query(
      `SELECT COUNT(*) as total FROM \`${table}\` ${filters}`,
      params
    );
    const total = countRows[0]?.total || 0;

    // Get filtered + paginated data
    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` ${filters} ORDER BY id DESC LIMIT ? OFFSET ?`,
      [...params, limit, offset]
    );

    res.status(200).json({
      data: rows,
      page,
      limit,
      total,
      totalPages: Math.ceil(total / limit),
    });
  } catch (err) {
    console.error("DB fetch error:", err.message || err);
    res.status(500).json({ error: "Database error" });
  }
});


module.exports = router;

const express = require("express");
const router = express.Router();
const { pool } = require("../db"); // Adjust path if needed

// GET latest 100 rows for S7-200
router.get("/getAllDataSmart200", async (req, res) => {
  try {
    const [rows] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 100"
    );
    res.status(200).json(rows);
  } catch (err) {
    console.error("DB fetch error:", err.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

// Add similar one for S7-1200
router.get("/getAllData", async (req, res) => {
  try {
    const [rows] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 100"
    );
    res.status(200).json(rows);
  } catch (err) {
    console.error("DB fetch error:", err.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

// GET paginated rows for kabomachinedatasmart200
router.get("/paginatedSmart200", async (req, res) => {
  let page = parseInt(req.query.page, 10) || 1;
  let limit = parseInt(req.query.limit, 10) || 10;
  const from = req.query.from; // expected format: 'YYYY-MM-DD'
  const to = req.query.to;     // expected format: 'YYYY-MM-DD'

  if (page < 1) page = 1;
  if (limit < 1) limit = 10;
  const offset = (page - 1) * limit;

  let filters = "";
  const params = [];

  // Handle date filtering conditionally
  if (from && to) {
    filters = "WHERE date_column BETWEEN ? AND ?";
    params.push(from, to);
  } else if (from) {
    filters = "WHERE date_column >= ?";
    params.push(from);
  } else if (to) {
    filters = "WHERE date_column <= ?";
    params.push(to);
  }

  try {
    // Get total filtered count
    const [countRows] = await pool.query(
      `SELECT COUNT(*) as total FROM kabomachinedatasmart200 ${filters}`,
      params
    );
    const total = countRows[0]?.total || 0;

    // Get filtered + paginated data
    const [rows] = await pool.query(
      `SELECT * FROM kabomachinedatasmart200 ${filters} ORDER BY id DESC LIMIT ? OFFSET ?`,
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
  const from = req.query.from; // expected format: 'YYYY-MM-DD'
  const to = req.query.to;     // expected format: 'YYYY-MM-DD'

  if (page < 1) page = 1;
  if (limit < 1) limit = 10;
  const offset = (page - 1) * limit;

  let filters = "";
  const params = [];

  // Handle date filtering conditionally
  if (from && to) {
    filters = "WHERE date_column BETWEEN ? AND ?";
    params.push(from, to);
  } else if (from) {
    filters = "WHERE date_column >= ?";
    params.push(from);
  } else if (to) {
    filters = "WHERE date_column <= ?";
    params.push(to);
  }

  try {
    // Get total filtered count
    const [countRows] = await pool.query(
      `SELECT COUNT(*) as total FROM gtpl_122_s7_1200_01 ${filters}`,
      params
    );
    const total = countRows[0]?.total || 0;

    // Get filtered + paginated data
    const [rows] = await pool.query(
      `SELECT * FROM kabomachinedatasmart200 ${filters} ORDER BY id DESC LIMIT ? OFFSET ?`,
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

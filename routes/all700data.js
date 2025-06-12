
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
      "SELECT * FROM kabomachinedata ORDER BY id DESC LIMIT 100"
    );
    res.status(200).json(rows);
  } catch (err) {
    console.error("DB fetch error:", err.message || err);
    res.status(500).json({ error: "Database error" });
  }
});

module.exports = router;

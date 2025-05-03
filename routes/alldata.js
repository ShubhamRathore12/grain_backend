const express = require("express");
const router = express.Router();
const { pool } = require("../db");

// 🚀 API to get last 100 records
router.get("/machine-data", async (req, res) => {
  try {
    const [rows] = await pool.query(
      "SELECT * FROM kab0machinedatasmart200 ORDER BY id DESC LIMIT 100"
    );
    res.status(200).json({
      success: true,
      data: rows,
      timestamp: new Date().toISOString(),
    });
  } catch (error) {
    console.error("Error fetching machine data:", error);
    res.status(500).json({
      success: false,
      error: "Failed to fetch machine data",
      message: error.message,
    });
  }
});

// 🚀 Optional: API to get only the very latest single record
router.get("/current-data", async (req, res) => {
  try {
    const [rows] = await pool.query(
      "SELECT * FROM kabumachinedata ORDER BY id DESC LIMIT 1"
    );
    res.status(200).json({
      success: true,
      data: rows[0] || null,
      timestamp: new Date().toISOString(),
    });
  } catch (error) {
    console.error("Error fetching current data:", error);
    res.status(500).json({
      success: false,
      error: "Failed to fetch current data",
      message: error.message,
    });
  }
});

module.exports = router;

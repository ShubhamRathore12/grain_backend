const express = require("express");
const router = express.Router();
const { pool } = require("../db");

// Middleware to handle CORS for SSE
const sseHeaders = (req, res, next) => {
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader("Access-Control-Allow-Headers", "Content-Type, Authorization");
  res.setHeader("Access-Control-Allow-Methods", "GET, OPTIONS");
  next();
};

// Handle preflight requests
router.options("/machine-data", (req, res) => {
  res.status(200).end();
});

// Main SSE endpoint
router.get("/machine-data", sseHeaders, async (req, res) => {
  // Send initial connection message
  res.write("retry: 2000\n");
  res.write('event: connected\ndata: {"status": "connected"}\n\n');

  let lastInsertedId = 0;

  const checkForUpdates = async () => {
    try {
      const [rows] = await pool.query(
        "SELECT * FROM kabumachinedata ORDER BY id DESC LIMIT 1"
      );
      const latest = rows[0];

      if (latest && latest.id > lastInsertedId) {
        lastInsertedId = latest.id;
        // Format data for React consumption
        const formattedData = {
          id: latest.id,
          timestamp: new Date().toISOString(),
          data: latest,
        };
        res.write(`event: update\ndata: ${JSON.stringify(formattedData)}\n\n`);
      }
    } catch (err) {
      console.error("DB fetch error:", err);
      res.write(
        `event: error\ndata: ${JSON.stringify({
          error: "DB error",
          message: err.message,
          timestamp: new Date().toISOString(),
        })}\n\n`
      );
    }
  };

  // Check for updates every 2 seconds
  const interval = setInterval(checkForUpdates, 2000);

  // Handle client disconnect
  req.on("close", () => {
    console.log("SSE client disconnected");
    clearInterval(interval);
    res.end();
  });
});

// Regular API endpoint to get current data
router.get("/current-data", async (req, res) => {
  try {
    const [rows] = await pool.query(
      "SELECT * FROM kabumachinedata ORDER BY id DESC LIMIT 1"
    );
    res.json({
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

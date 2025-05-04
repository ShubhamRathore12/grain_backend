const express = require("express");
const router = express.Router();
const { pool } = require("../db");

// Function to broadcast data to all connected clients
function broadcastData(wss, data) {
  wss.clients.forEach((client) => {
    if (client.readyState === require("ws").OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// Regular API endpoint to get current data
router.get("/current-data", async (req, res) => {
  try {
    const [rows] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
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

// Function to check for new data and broadcast
async function checkAndBroadcastData(wss) {
  try {
    const [rows] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );
    const latest = rows[0];

    if (latest) {
      const formattedData = {
        type: "update",
        data: latest,
        timestamp: new Date().toISOString(),
      };
      broadcastData(wss, formattedData);
    }
  } catch (err) {
    console.error("DB fetch error:", err);
    broadcastData(wss, {
      type: "error",
      error: "DB error",
      message: err.message,
      timestamp: new Date().toISOString(),
    });
  }
}

module.exports = {
  router,
  checkAndBroadcastData,
};

const express = require("express");
const router = express.Router();
const { pool } = require("../db");
const WebSocket = require("ws");

// Utility: Broadcast to all WebSocket clients
function broadcastData(wss, data) {
  wss.clients.forEach((client) => {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// GET latest row from kabumachinedata table
router.get("/alldata", async (req, res) => {
  try {
    const [rows] = await pool.query(
      `SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1`
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

// Function to broadcast latest data over WebSocket
async function checkAndBroadcastData(wss) {
  try {
    const [rows] = await pool.query(
      `SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1`
    );

    const latest = rows[0];

    if (latest) {
      broadcastData(wss, {
        type: "update",
        data: latest,
        timestamp: new Date().toISOString(),
      });
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


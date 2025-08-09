const express = require("express");
const router = express.Router();
const { safeQuery } = require("../db");
const WebSocket = require("ws");

// Utility: Broadcast to all WebSocket clients
function broadcastData(wss, data) {
  wss.clients.forEach((client) => {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// GET latest row from gtpl_122_s7_1200_01 table
router.get("/alldata", async (req, res) => {
  try {
    const rows = await safeQuery(
      `SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1`
    );

    res.json({
      success: true,
      data: rows[0] || null,
      timestamp: new Date().toISOString(),
    });
  } catch (error) {
    console.error("Error fetching current data:", error.message);

    if (
      error.message.includes("Database connection unavailable") ||
      error.message.includes("ETIMEDOUT")
    ) {
      return res.status(503).json({
        success: false,
        error: "Database service temporarily unavailable",
        message: "Please try again later",
      });
    }

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
    const rows = await safeQuery(
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
    console.error("DB fetch error:", err.message);

    // Only broadcast error if it's not a connection timeout
    if (
      !err.message.includes("Database connection unavailable") &&
      !err.message.includes("ETIMEDOUT")
    ) {
      broadcastData(wss, {
        type: "error",
        error: "DB error",
        message: err.message,
        timestamp: new Date().toISOString(),
      });
    }
  }
}

module.exports = {
  router,
  checkAndBroadcastData,
};

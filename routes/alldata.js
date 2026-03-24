const express = require("express");
const router = express.Router();
const { safeQuery } = require("../db");
const WebSocket = require("ws");
const { MACHINE_CONFIG } = require("../utils/machineConfig");

const allowedTables = new Set(
  Object.values(MACHINE_CONFIG).map((c) => c.table)
);

// Utility: Broadcast to all WebSocket clients
function broadcastData(wss, data) {
  wss.clients.forEach((client) => {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// GET latest row from any allowed table
router.get("/alldata", async (req, res) => {
  try {
    const table = req.query.table || "kabomachinedatasmart200";

    if (!allowedTables.has(table)) {
      return res.status(400).json({
        success: false,
        error: "Invalid table name",
        allowedTables: Array.from(allowedTables),
      });
    }

    const rows = await safeQuery(
      `SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 1`
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
async function checkAndBroadcastData(wss, table = "kabomachinedatasmart200") {
  try {
    const rows = await safeQuery(
      `SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 1`
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

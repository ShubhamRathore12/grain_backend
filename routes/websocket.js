const express = require("express");
const router = express.Router();
const { pool } = require("../db");

// Utility: determine table by product
function getTableName(product) {
  switch (product) {
    case 1:
      return "kabumachinedata";
    case 2:
      return "kabomachinedatasmart200";
    default:
      return "kabumachinedata"; // fallback
  }
}

// Broadcast to all clients
function broadcastData(wss, data) {
  wss.clients.forEach((client) => {
    if (client.readyState === require("ws").OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// GET current data from specific table
router.get("/current-data", async (req, res) => {
  const product = req.query.product || "s7-1200";
  const table = getTableName(product);

  try {
    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 1`
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

// Check and broadcast for WebSocket
async function checkAndBroadcastData(wss, product = "s7-1200") {
  const table = getTableName(product);

  try {
    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 1`
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

const express = require("express");
const router = express.Router();
const { pool } = require("../db");

// Always return default table
function getTableName() {
  return "kabumachinedata";
}

// Broadcast data to all connected clients
function broadcastData(wss, data) {
  wss.clients.forEach((client) => {
    if (client.readyState === require("ws").OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// GET /api/ws/current-data — fetch latest row from default table
router.get("/current-data", async (req, res) => {
  const table = getTableName(); // default table

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

// Periodically fetch data and broadcast via WebSocket
async function checkAndBroadcastData(wss) {
  const table = getTableName(); // always default

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

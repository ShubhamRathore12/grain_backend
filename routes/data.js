const express = require("express");
const router = express.Router();
const { authenticateToken } = require("../middleware/auth");

// Function to broadcast data to all connected clients
function broadcastData(wss, data) {
  wss.clients.forEach((client) => {
    if (client.readyState === require("ws").OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// Protected route that triggers real-time updates
router.post("/update", authenticateToken, async (req, res) => {
  try {
    const newData = req.body;

    // After successful update, broadcast to all clients
    broadcastData(req.app.get("wss"), {
      type: "data_update",
      data: newData,
      timestamp: new Date().toISOString(),
    });

    res.json({ message: "Data updated successfully" });
  } catch (error) {
    console.error("Update error:", error);
    res.status(500).json({ message: "Error updating data" });
  }
});

module.exports = router;

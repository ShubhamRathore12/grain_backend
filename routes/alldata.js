// routes/sse.js
const express = require("express");
const router = express.Router();
const { pool } = require("../db");

router.get("/alldata", async (req, res) => {
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");

  // Let the frontend know the connection was successful
  res.write(`retry: 2000\n`);
  res.write(`event: connected\ndata: connected\n\n`);

  let lastInsertedId = 0;

  const interval = setInterval(async () => {
    try {
      const [rows] = await pool.query(
        "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
      );
      const latest = rows[0];

      if (latest && latest.id > lastInsertedId) {
        lastInsertedId = latest.id;

        // 🛠 Structure with `success` and `data`
        const payload = {
          success: true,
          data: latest,
        };

        res.write(`event: message\ndata: ${JSON.stringify(payload)}\n\n`);
      }
    } catch (err) {
      console.error("DB fetch error:", err?.message || err);

      res.write(
        `event: error\ndata: ${JSON.stringify({
          success: false,
          error: err?.message || "DB error",
        })}\n\n`
      );
    }
  }, 2000);

  req.on("close", () => {
    clearInterval(interval);
    res.end();
    console.log("SSE client disconnected");
  });
});

module.exports = router;

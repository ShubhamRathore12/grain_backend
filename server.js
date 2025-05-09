const express = require("express");
const http = require("http");
const WebSocket = require("ws");
const cors = require("cors");
const jwt = require("jsonwebtoken");
const { pool } = require("./db");
const path = require("path");
require("dotenv").config();

// Import routes
const authRoutes = require("./routes/auth");
const dataRoutes = require("./routes/data");
const alldataRoutes = require("./routes/alldata");

const {
  router: websocketRoutes,
  checkAndBroadcastData,
} = require("./routes/websocket");
const registerRoutes = require("./routes/register");

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

// Store WebSocket server instance in app for use in routes
app.set("wss", wss);

app.use(cors());
app.use(express.json());
app.use(express.static(path.join(__dirname, "public")));

// WebSocket connection handling
wss.on("connection", (ws) => {
  console.log("New client connected");

  // Send initial connection message
  ws.send(
    JSON.stringify({
      type: "connected",
      data: { status: "connected" },
      timestamp: new Date().toISOString(),
    })
  );

  ws.on("message", (message) => {
    console.log("Received:", message);
    // Handle incoming messages if needed
  });

  ws.on("close", () => {
    console.log("Client disconnected");
  });

  ws.on("error", (error) => {
    console.error("WebSocket error:", error);
  });
});

// Check for new data every 2 seconds
setInterval(() => {
  checkAndBroadcastData(wss);
}, 2000);

// Login route
app.post("/api/login", async (req, res) => {
  const { username, password } = req.body;

  try {
    const [rows] = await pool.query(
      "SELECT * FROM kabu_users WHERE username = ? AND password = ?",
      [username, password]
    );

    if (!rows || rows.length === 0) {
      return res.status(401).json({ message: "Invalid username or password" });
    }

    const user = rows[0];
    const token = jwt.sign(
      {
        username: user.username,
        accountType: user.accountType,
        userId: user.id,
      },
      process.env.JWT_SECRET,
      { expiresIn: "15m" }
    );

    // Remove password from response
    const { password: _, ...userWithoutPassword } = user;

    res.cookie("auth_token", token, {
      httpOnly: true,
      sameSite: "lax",
      maxAge: 15 * 60 * 1000, // 15 minutes
      path: "/dashboard",
    });

    res.json({
      message: "Login successful",
      user: userWithoutPassword,
    });
  } catch (error) {
    console.error("Login error:", error);
    res.status(500).json({ message: "Server error while logging in" });
  }
});

// Example route that triggers real-time updates
app.post("/api/update-data", async (req, res) => {
  try {
    // Your database update logic here
    const newData = req.body;

    // After successful update, broadcast to all clients
    broadcastData({
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

// Routes
app.use("/api/auth", authRoutes);
app.use("/api/data", dataRoutes);
app.use("/api/alldata", alldataRoutes);
app.use("/api/ws", websocketRoutes);
app.use("/api/register", registerRoutes);

// Error handling middleware
app.use((err, req, res, next) => {
  console.error(err.stack);
  res.status(500).json({ message: "Something broke!" });
});

const PORT = process.env.PORT || 3000;
server.listen(PORT, () => {
  console.log(`Server running on port ${PORT}`);
});

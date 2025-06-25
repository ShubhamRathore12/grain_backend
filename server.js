const express = require("express");
const http = require("http");
const WebSocket = require("ws");
const cors = require("cors");
const jwt = require("jsonwebtoken");
const { pool } = require("./db");
const path = require("path");
require("dotenv").config();

// Import database initialization
const { initializeDatabase, safeQuery } = require("./db");

// Import routes
const authRoutes = require("./routes/auth");
const dataRoutes = require("./routes/data");
const alldataRoutes = require("./routes/alldata").router;
const {
  router: websocketRoutes,
  checkAndBroadcastData,
} = require("./routes/websocket");
const registerRoutes = require("./routes/register");
const dataRouters = require("./routes/all700data");
const {
  router: machineStatusRoutes,
  checkAndBroadcastMachineStatus,
  startTimeoutReset,
} = require("./routes/machineStatus");

const app = express();
const server = http.createServer(app);

// Configure CORS
const corsOptions = {
  origin: process.env.CORS_ORIGIN || "*",
  methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"],
  allowedHeaders: ["Content-Type", "Authorization"],
  credentials: true,
};

app.use(cors(corsOptions));
app.use(express.json());
app.use(express.static(path.join(__dirname, "public")));

// WebSocket server with error handling
const wss = new WebSocket.Server({
  server,
  path: "/ws",
  clientTracking: true,
  handleProtocols: () => true,
});

// Store WebSocket server instance in app for use in routes
app.set("wss", wss);

// Utility: Broadcast to all WebSocket clients
function broadcastData(data) {
  wss.clients.forEach((client) => {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// WebSocket connection handling with better error handling
wss.on("connection", (ws, req) => {
  console.log("New client connected");

  // Send initial connection message
  try {
    ws.send(
      JSON.stringify({
        type: "connected",
        data: { status: "connected" },
        timestamp: new Date().toISOString(),
      })
    );
  } catch (error) {
    console.error("Error sending initial message:", error);
  }

  ws.on("message", (message) => {
    try {
      console.log("Received:", message.toString());
      // Handle incoming messages if needed
    } catch (error) {
      console.error("Error processing message:", error);
    }
  });

  ws.on("close", () => {
    console.log("Client disconnected");
  });

  ws.on("error", (error) => {
    console.error("WebSocket error:", error);
  });
});

// Check for new data every 2 seconds with error handling
const dataCheckInterval = setInterval(() => {
  try {
    checkAndBroadcastData(wss);
  } catch (error) {
    console.error("Error checking and broadcasting data:", error);
  }
}, 2000);

// Check for machine status updates every 3 seconds
const machineStatusInterval = setInterval(() => {
  try {
    checkAndBroadcastMachineStatus(wss);
  } catch (error) {
    console.error("Error checking and broadcasting machine status:", error);
  }
}, 3000);

// Initialize timeout reset for machine status
startTimeoutReset(wss);

// Cleanup on server shutdown
process.on("SIGTERM", () => {
  clearInterval(dataCheckInterval);
  clearInterval(machineStatusInterval);
  wss.close(() => {
    console.log("WebSocket server closed");
    process.exit(0);
  });
});

// Login route
app.post("/api/login", async (req, res) => {
  const { username, password } = req.body;

  try {
    const rows = await safeQuery(
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
    console.error("Login error:", error.message);

    if (
      error.message.includes("Database connection unavailable") ||
      error.message.includes("ETIMEDOUT")
    ) {
      return res.status(503).json({
        message:
          "Database service temporarily unavailable. Please try again later.",
      });
    }

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
app.use("/api/all700data", dataRouters);
app.use("/api/machine", machineStatusRoutes);

// Health check endpoint
app.get("/api/health", async (req, res) => {
  const { isDatabaseConnected } = require("./db");

  try {
    const dbStatus = isDatabaseConnected();
    res.json({
      status: "ok",
      timestamp: new Date().toISOString(),
      database: dbStatus ? "connected" : "disconnected",
      environment: process.env.NODE_ENV || "development",
    });
  } catch (error) {
    res.status(503).json({
      status: "error",
      timestamp: new Date().toISOString(),
      database: "error",
      error: error.message,
    });
  }
});

// Error handling middleware
app.use((err, req, res, next) => {
  console.error(err.stack);
  res.status(500).json({
    message:
      process.env.NODE_ENV === "production"
        ? "Internal server error"
        : err.message,
  });
});

const PORT = process.env.PORT || 3000;

// Initialize database and start server
async function startServer() {
  try {
    console.log("Starting server initialization...");
    await initializeDatabase();
    server.listen(PORT, () => {
      console.log(
        `✅ Server running on port ${PORT} in ${
          process.env.NODE_ENV || "development"
        } mode`
      );
    });
  } catch (error) {
    console.error("❌ Failed to start server:", error.message);

    if (error.message.includes("Failed to connect to database")) {
      console.log("\n🔧 Database Connection Issues:");
      console.log(
        "1. Check if your database server allows external connections"
      );
      console.log(
        "2. Verify environment variables are set correctly in Render"
      );
      console.log("3. Consider using a cloud database service for production");
      console.log(
        "4. Contact your hosting provider to allow Render's IP addresses"
      );
    }

    // For now, start the server anyway (without database)
    console.log("\n⚠️  Starting server without database connection...");
    server.listen(PORT, () => {
      console.log(
        `⚠️  Server running on port ${PORT} (database connection failed)`
      );
    });
  }
}

startServer();

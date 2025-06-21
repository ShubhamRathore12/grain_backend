const express = require("express");
const router = express.Router();
const { pool } = require("../db");
const { authenticateToken } = require("../middleware/auth");
const WebSocket = require("ws");

// Store last known data for comparison
let lastGtplData = null;
let lastKaboData = null;
let lastUpdateTime = null;
let timeoutInterval = null;

// Utility: Broadcast to all WebSocket clients
function broadcastData(wss, data) {
  if (wss && wss.clients) {
    wss.clients.forEach((client) => {
      if (client.readyState === WebSocket.OPEN) {
        client.send(JSON.stringify(data));
      }
    });
  }
}

// Function to check if data has actually changed (not just ID)
function hasDataChanged(newData, lastData) {
  if (!lastData) return true;

  // Compare all fields except id and timestamp fields
  const fieldsToCompare = Object.keys(newData).filter(
    (key) => !["id", "created_at", "timestamp", "updated_at"].includes(key)
  );

  for (const field of fieldsToCompare) {
    if (newData[field] !== lastData[field]) {
      return true;
    }
  }

  return false;
}

// Function to reset values to 0 after 18 seconds of no updates
function startTimeoutReset(wss) {
  // Clear existing timeout
  if (timeoutInterval) {
    clearTimeout(timeoutInterval);
  }

  timeoutInterval = setTimeout(() => {
    console.log(
      "18 seconds passed without data updates, resetting values to 0"
    );

    const resetData = {
      success: true,
      message: "Values reset to 0 due to no updates",
      data: {
        machineStatus: false,
        coolingStatus: false,
        internetStatus: false,
        lastUpdate: {
          gtpl: new Date().toISOString(),
          kabo: new Date().toISOString(),
        },
        recordIds: {
          gtpl: lastGtplData?.id || 0,
          kabo: lastKaboData?.id || 0,
        },
        reset: true,
      },
      timestamp: new Date().toISOString(),
    };

    // Broadcast reset data
    broadcastData(wss, {
      type: "machine_status_reset",
      data: resetData,
      timestamp: new Date().toISOString(),
    });

    // Reset stored data
    lastGtplData = null;
    lastKaboData = null;
    lastUpdateTime = null;
  }, 18000); // 18 seconds
}

// Function to check and broadcast machine status
async function checkAndBroadcastMachineStatus(wss) {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table
    const [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table
    const [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    // Check if data exists
    if (
      !gtplData ||
      gtplData.length === 0 ||
      !kaboData ||
      kaboData.length === 0
    ) {
      const noDataResponse = {
        success: false,
        message: "No machine data found",
        data: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          lastUpdate: {
            gtpl: null,
            kabo: null,
          },
          recordIds: {
            gtpl: 0,
            kabo: 0,
          },
        },
        timestamp: new Date().toISOString(),
      };

      broadcastData(wss, {
        type: "machine_status",
        data: noDataResponse,
        timestamp: new Date().toISOString(),
      });

      return;
    }

    const gtplRecord = gtplData[0];
    const kaboRecord = kaboData[0];

    // Check if data has actually changed
    const gtplChanged = hasDataChanged(gtplRecord, lastGtplData);
    const kaboChanged = hasDataChanged(kaboRecord, lastKaboData);

    // Only proceed if data has changed or it's the first time
    if (gtplChanged || kaboChanged || !lastUpdateTime) {
      // Update stored data
      lastGtplData = gtplRecord;
      lastKaboData = kaboRecord;
      lastUpdateTime = new Date();

      // Restart timeout
      startTimeoutReset(wss);

      // Get current timestamp
      const currentTime = new Date();

      // Check if data is recent (within last 5 minutes for machine status)
      const gtplTimestamp = new Date(
        gtplRecord.created_at || gtplRecord.timestamp || currentTime
      );
      const kaboTimestamp = new Date(
        kaboRecord.created_at || kaboRecord.timestamp || currentTime
      );

      const fiveMinutesAgo = new Date(currentTime.getTime() - 5 * 60 * 1000);
      const oneMinuteAgo = new Date(currentTime.getTime() - 1 * 60 * 1000);

      // Determine machine status based on recent data updates
      const machineStatus =
        gtplTimestamp > fiveMinutesAgo && kaboTimestamp > fiveMinutesAgo;

      // Determine cooling status
      let coolingStatus = false;
      if (gtplRecord.cooling_status !== undefined) {
        coolingStatus = gtplRecord.cooling_status;
      } else if (gtplRecord.cooling !== undefined) {
        coolingStatus = gtplRecord.cooling;
      } else if (kaboRecord.cooling_status !== undefined) {
        coolingStatus = kaboRecord.cooling_status;
      } else if (kaboRecord.cooling !== undefined) {
        coolingStatus = kaboRecord.cooling;
      } else {
        // Default cooling status based on recent data
        coolingStatus =
          gtplTimestamp > oneMinuteAgo || kaboTimestamp > oneMinuteAgo;
      }

      // Determine internet status based on data freshness
      const internetStatus =
        gtplTimestamp > oneMinuteAgo || kaboTimestamp > oneMinuteAgo;

      // Prepare response
      const response = {
        success: true,
        message: "Machine status updated",
        data: {
          machineStatus,
          coolingStatus,
          internetStatus,
          lastUpdate: {
            gtpl: gtplTimestamp.toISOString(),
            kabo: kaboTimestamp.toISOString(),
          },
          recordIds: {
            gtpl: gtplRecord.id,
            kabo: kaboRecord.id,
          },
          dataChanged: {
            gtpl: gtplChanged,
            kabo: kaboChanged,
          },
        },
        timestamp: currentTime.toISOString(),
      };

      // Broadcast the update
      broadcastData(wss, {
        type: "machine_status_update",
        data: response,
        timestamp: currentTime.toISOString(),
      });
    }
  } catch (error) {
    console.error("Error checking machine status:", error);

    const errorResponse = {
      success: false,
      message: "Error checking machine status",
      error:
        process.env.NODE_ENV === "development"
          ? error.message
          : "Internal server error",
      data: {
        machineStatus: false,
        coolingStatus: false,
        internetStatus: false,
      },
      timestamp: new Date().toISOString(),
    };

    broadcastData(wss, {
      type: "machine_status_error",
      data: errorResponse,
      timestamp: new Date().toISOString(),
    });
  }
}

// Machine Status API
router.get("/status", authenticateToken, async (req, res) => {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table
    const [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table
    const [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    // Check if data exists
    if (
      !gtplData ||
      gtplData.length === 0 ||
      !kaboData ||
      kaboData.length === 0
    ) {
      return res.status(404).json({
        success: false,
        message: "No machine data found",
        machineStatus: false,
        coolingStatus: false,
        internetStatus: false,
      });
    }

    const gtplRecord = gtplData[0];
    const kaboRecord = kaboData[0];

    // Get current timestamp
    const currentTime = new Date();

    // Check if data is recent (within last 5 minutes for machine status)
    const gtplTimestamp = new Date(
      gtplRecord.created_at || gtplRecord.timestamp || currentTime
    );
    const kaboTimestamp = new Date(
      kaboRecord.created_at || kaboRecord.timestamp || currentTime
    );

    const fiveMinutesAgo = new Date(currentTime.getTime() - 5 * 60 * 1000);
    const oneMinuteAgo = new Date(currentTime.getTime() - 1 * 60 * 1000);

    // Determine machine status based on recent data updates
    const machineStatus =
      gtplTimestamp > fiveMinutesAgo && kaboTimestamp > fiveMinutesAgo;

    // Determine cooling status
    let coolingStatus = false;
    if (gtplRecord.cooling_status !== undefined) {
      coolingStatus = gtplRecord.cooling_status;
    } else if (gtplRecord.cooling !== undefined) {
      coolingStatus = gtplRecord.cooling;
    } else if (kaboRecord.cooling_status !== undefined) {
      coolingStatus = kaboRecord.cooling_status;
    } else if (kaboRecord.cooling !== undefined) {
      coolingStatus = kaboRecord.cooling;
    } else {
      // Default cooling status based on recent data
      coolingStatus =
        gtplTimestamp > oneMinuteAgo || kaboTimestamp > oneMinuteAgo;
    }

    // Determine internet status based on data freshness
    const internetStatus =
      gtplTimestamp > oneMinuteAgo || kaboTimestamp > oneMinuteAgo;

    // Check if data has changed
    const gtplChanged = hasDataChanged(gtplRecord, lastGtplData);
    const kaboChanged = hasDataChanged(kaboRecord, lastKaboData);

    // Prepare response
    const response = {
      success: true,
      message: "Machine status retrieved successfully",
      data: {
        machineStatus,
        coolingStatus,
        internetStatus,
        lastUpdate: {
          gtpl: gtplTimestamp.toISOString(),
          kabo: kaboTimestamp.toISOString(),
        },
        recordIds: {
          gtpl: gtplRecord.id,
          kabo: kaboRecord.id,
        },
        dataChanged: {
          gtpl: gtplChanged,
          kabo: kaboChanged,
        },
      },
      timestamp: currentTime.toISOString(),
    };

    res.json(response);
  } catch (error) {
    console.error("Error fetching machine status:", error);
    res.status(500).json({
      success: false,
      message: "Error fetching machine status",
      error:
        process.env.NODE_ENV === "development"
          ? error.message
          : "Internal server error",
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
    });
  }
});

// Alternative endpoint without authentication for monitoring purposes
router.get("/status/public", async (req, res) => {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table
    const [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table
    const [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    // Check if data exists
    if (
      !gtplData ||
      gtplData.length === 0 ||
      !kaboData ||
      kaboData.length === 0
    ) {
      return res.status(404).json({
        success: false,
        message: "No machine data found",
        machineStatus: false,
        coolingStatus: false,
        internetStatus: false,
      });
    }

    const gtplRecord = gtplData[0];
    const kaboRecord = kaboData[0];

    // Get current timestamp
    const currentTime = new Date();

    // Check if data is recent (within last 5 minutes for machine status)
    const gtplTimestamp = new Date(
      gtplRecord.created_at || gtplRecord.timestamp || currentTime
    );
    const kaboTimestamp = new Date(
      kaboRecord.created_at || kaboRecord.timestamp || currentTime
    );

    const fiveMinutesAgo = new Date(currentTime.getTime() - 5 * 60 * 1000);
    const oneMinuteAgo = new Date(currentTime.getTime() - 1 * 60 * 1000);

    // Determine machine status based on recent data updates
    const machineStatus =
      gtplTimestamp > fiveMinutesAgo && kaboTimestamp > fiveMinutesAgo;

    // Determine cooling status
    let coolingStatus = false;
    if (gtplRecord.cooling_status !== undefined) {
      coolingStatus = gtplRecord.cooling_status;
    } else if (gtplRecord.cooling !== undefined) {
      coolingStatus = gtplRecord.cooling;
    } else if (kaboRecord.cooling_status !== undefined) {
      coolingStatus = kaboRecord.cooling_status;
    } else if (kaboRecord.cooling !== undefined) {
      coolingStatus = kaboRecord.cooling;
    } else {
      // Default cooling status based on recent data
      coolingStatus =
        gtplTimestamp > oneMinuteAgo || kaboTimestamp > oneMinuteAgo;
    }

    // Determine internet status based on data freshness
    const internetStatus =
      gtplTimestamp > oneMinuteAgo || kaboTimestamp > oneMinuteAgo;

    // Check if data has changed
    const gtplChanged = hasDataChanged(gtplRecord, lastGtplData);
    const kaboChanged = hasDataChanged(kaboRecord, lastKaboData);

    // Prepare response
    const response = {
      success: true,
      message: "Machine status retrieved successfully",
      data: {
        machineStatus,
        coolingStatus,
        internetStatus,
        lastUpdate: {
          gtpl: gtplTimestamp.toISOString(),
          kabo: kaboTimestamp.toISOString(),
        },
        recordIds: {
          gtpl: gtplRecord.id,
          kabo: kaboRecord.id,
        },
        dataChanged: {
          gtpl: gtplChanged,
          kabo: kaboChanged,
        },
      },
      timestamp: currentTime.toISOString(),
    };

    res.json(response);
  } catch (error) {
    console.error("Error fetching machine status:", error);
    res.status(500).json({
      success: false,
      message: "Error fetching machine status",
      error:
        process.env.NODE_ENV === "development"
          ? error.message
          : "Internal server error",
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
    });
  }
});

// Test endpoint to see raw data from both tables
router.get("/test", async (req, res) => {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table
    const [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table
    const [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    res.json({
      success: true,
      message: "Raw data from both tables",
      gtplData: gtplData.length > 0 ? gtplData[0] : null,
      kaboData: kaboData.length > 0 ? kaboData[0] : null,
      timestamp: new Date().toISOString(),
    });
  } catch (error) {
    console.error("Error fetching test data:", error);
    res.status(500).json({
      success: false,
      message: "Error fetching test data",
      error:
        process.env.NODE_ENV === "development"
          ? error.message
          : "Internal server error",
    });
  }
});

module.exports = {
  router,
  checkAndBroadcastMachineStatus,
  startTimeoutReset,
};

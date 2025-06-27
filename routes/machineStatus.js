const express = require("express");
const router = express.Router();
const { pool } = require("../db");
const { authenticateToken } = require("../middleware/auth");
const WebSocket = require("ws");

// Store last known timestamps and IDs for comparison
let lastGtplTimestamp = null;
let lastKaboTimestamp = null;
let lastGtplId = null;
let lastKaboId = null;
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

// Function to check if timestamp has actually changed
function hasTimestampChanged(newTimestamp, lastTimestamp) {
  if (!lastTimestamp) return true;

  // Convert to Date objects for comparison
  const newDate = new Date(newTimestamp);
  const lastDate = new Date(lastTimestamp);

  // Check if the new timestamp is different (and newer)
  return newDate.getTime() !== lastDate.getTime();
}

// Function to check if ID has changed (indicating new data)
function hasIdChanged(newId, lastId) {
  if (!lastId) return true;
  return newId !== lastId;
}

// Function to get machine-specific response based on data freshness and ID changes
function getMachineSpecificResponse(
  machineType,
  timestamp,
  currentTime,
  hasNewData
) {
  const timestampDate = new Date(timestamp);
  const fiveMinutesAgo = new Date(currentTime.getTime() - 5 * 60 * 1000);
  const oneMinuteAgo = new Date(currentTime.getTime() - 1 * 60 * 1000);
  const thirtySecondsAgo = new Date(currentTime.getTime() - 30 * 1000);

  // If no new data (same ID), return false for all statuses
  if (!hasNewData) {
    return {
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
      machineType:
        machineType === "gtpl"
          ? "GTPL_122_S7_1200_01"
          : "KABO_MACHINE_SMART200",
      priority: machineType === "gtpl" ? "high" : "medium",
      responseType: machineType === "gtpl" ? "gtpl_machine" : "kabo_machine",
      noNewData: true,
    };
  }

  // Different logic for different machines
  switch (machineType) {
    case "gtpl":
      return {
        machineStatus: timestampDate > fiveMinutesAgo,
        coolingStatus: timestampDate > oneMinuteAgo,
        internetStatus: timestampDate > thirtySecondsAgo,
        machineType: "GTPL_122_S7_1200_01",
        priority: "high",
        responseType: "gtpl_machine",
        noNewData: false,
      };

    case "kabo":
      return {
        machineStatus: timestampDate > fiveMinutesAgo,
        coolingStatus: timestampDate > oneMinuteAgo,
        internetStatus: timestampDate > thirtySecondsAgo,
        machineType: "KABO_MACHINE_SMART200",
        priority: "medium",
        responseType: "kabo_machine",
        noNewData: false,
      };

    default:
      return {
        machineStatus: timestampDate > fiveMinutesAgo,
        coolingStatus: timestampDate > oneMinuteAgo,
        internetStatus: timestampDate > thirtySecondsAgo,
        machineType: "UNKNOWN",
        priority: "low",
        responseType: "unknown_machine",
        noNewData: false,
      };
  }
}

// Function to reset values to 0 after 18 seconds of no updates
function startTimeoutReset(wss) {
  // Clear existing timeout
  if (timeoutInterval) {
    clearTimeout(timeoutInterval);
  }

  timeoutInterval = setTimeout(() => {
    console.log(
      "18 seconds passed without timestamp updates, resetting values to 0"
    );

    const resetData = {
      success: true,
      message: "Values reset to 0 due to no timestamp updates",
      data: {
        gtpl: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "GTPL_122_S7_1200_01",
          priority: "high",
          responseType: "gtpl_machine",
          lastUpdate: new Date().toISOString(),
          reset: true,
          noNewData: true,
        },
        kabo: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "KABO_MACHINE_SMART200",
          priority: "medium",
          responseType: "kabo_machine",
          lastUpdate: new Date().toISOString(),
          reset: true,
          noNewData: true,
        },
      },
      timestamp: new Date().toISOString(),
    };

    // Broadcast reset data
    broadcastData(wss, {
      type: "machine_status_reset",
      data: resetData,
      timestamp: new Date().toISOString(),
    });

    // Reset stored timestamps and IDs
    lastGtplTimestamp = null;
    lastKaboTimestamp = null;
    lastGtplId = null;
    lastKaboId = null;
    lastUpdateTime = null;
  }, 18000); // 18 seconds
}

// Function to check and broadcast machine status
async function checkAndBroadcastMachineStatus(wss) {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table with safer ordering
    let [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table with safer ordering
    let [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    const currentTime = new Date();
    let hasAnyUpdate = false;

    // Process GTPL machine data
    let gtplResponse = null;
    if (gtplData && gtplData.length > 0) {
      const gtplRecord = gtplData[0];
      // Use the actual column names from your database
      const gtplTimestamp =
        gtplRecord.created_on ||
        gtplRecord.timestamp ||
        gtplRecord.updated_at ||
        gtplRecord.date_created ||
        currentTime;

      // Check if timestamp has changed
      const gtplTimestampChanged = hasTimestampChanged(
        gtplTimestamp,
        lastGtplTimestamp
      );
      // Check if ID has changed (new data)
      const gtplIdChanged = hasIdChanged(gtplRecord.id, lastGtplId);

      // Only update if either timestamp or ID has changed
      if (gtplTimestampChanged || gtplIdChanged) {
        lastGtplTimestamp = gtplTimestamp;
        lastGtplId = gtplRecord.id;
        hasAnyUpdate = true;

        gtplResponse = {
          ...getMachineSpecificResponse(
            "gtpl",
            gtplTimestamp,
            currentTime,
            gtplIdChanged
          ),
          recordId: gtplRecord.id,
          lastUpdate: new Date(gtplTimestamp).toISOString(),
          timestampChanged: gtplTimestampChanged,
          idChanged: gtplIdChanged,
          hasNewData: gtplIdChanged,
        };
      }
    }

    // Process KABO machine data
    let kaboResponse = null;
    if (kaboData && kaboData.length > 0) {
      const kaboRecord = kaboData[0];
      // Use the actual column names from your database
      const kaboTimestamp =
        kaboRecord.created_on ||
        kaboRecord.timestamp ||
        kaboRecord.updated_at ||
        kaboRecord.date_created ||
        currentTime;

      // Check if timestamp has changed
      const kaboTimestampChanged = hasTimestampChanged(
        kaboTimestamp,
        lastKaboTimestamp
      );
      // Check if ID has changed (new data)
      const kaboIdChanged = hasIdChanged(kaboRecord.id, lastKaboId);

      // Only update if either timestamp or ID has changed
      if (kaboTimestampChanged || kaboIdChanged) {
        lastKaboTimestamp = kaboTimestamp;
        lastKaboId = kaboRecord.id;
        hasAnyUpdate = true;

        kaboResponse = {
          ...getMachineSpecificResponse(
            "kabo",
            kaboTimestamp,
            currentTime,
            kaboIdChanged
          ),
          recordId: kaboRecord.id,
          lastUpdate: new Date(kaboTimestamp).toISOString(),
          timestampChanged: kaboTimestampChanged,
          idChanged: kaboIdChanged,
          hasNewData: kaboIdChanged,
        };
      }
    }

    // Only broadcast if there are updates or it's the first time
    if (hasAnyUpdate || (!lastGtplTimestamp && !lastKaboTimestamp)) {
      lastUpdateTime = currentTime;

      // Restart timeout
      startTimeoutReset(wss);

      // Prepare response with different JSON structure for each machine
      const response = {
        success: true,
        message: "Machine status updated based on timestamp and ID changes",
        data: {
          gtpl: gtplResponse || {
            machineStatus: false,
            coolingStatus: false,
            internetStatus: false,
            machineType: "GTPL_122_S7_1200_01",
            priority: "high",
            responseType: "gtpl_machine",
            lastUpdate: null,
            timestampChanged: false,
            idChanged: false,
            hasNewData: false,
            noNewData: true,
          },
          kabo: kaboResponse || {
            machineStatus: false,
            coolingStatus: false,
            internetStatus: false,
            machineType: "KABO_MACHINE_SMART200",
            priority: "medium",
            responseType: "kabo_machine",
            lastUpdate: null,
            timestampChanged: false,
            idChanged: false,
            hasNewData: false,
            noNewData: true,
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
        gtpl: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "GTPL_122_S7_1200_01",
          priority: "high",
          responseType: "gtpl_machine",
          error: true,
        },
        kabo: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "KABO_MACHINE_SMART200",
          priority: "medium",
          responseType: "kabo_machine",
          error: true,
        },
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
router.get("/status", async (req, res) => {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table with safer ordering
    const [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table with safer ordering
    const [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    const currentTime = new Date();

    // Process GTPL machine data
    let gtplResponse = {
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
      machineType: "GTPL_122_S7_1200_01",
      priority: "high",
      responseType: "gtpl_machine",
      lastUpdate: null,
      recordId: 0,
      hasNewData: false,
      noNewData: true,
    };

    if (gtplData && gtplData.length > 0) {
      const gtplRecord = gtplData[0];
      // Use the actual column names from your database
      const gtplTimestamp =
        gtplRecord.created_on ||
        gtplRecord.timestamp ||
        gtplRecord.updated_at ||
        gtplRecord.date_created ||
        currentTime;

      // Check if ID has changed (new data)
      const gtplIdChanged = hasIdChanged(gtplRecord.id, lastGtplId);

      gtplResponse = {
        ...getMachineSpecificResponse(
          "gtpl",
          gtplTimestamp,
          currentTime,
          gtplIdChanged
        ),
        recordId: gtplRecord.id,
        lastUpdate: new Date(gtplTimestamp).toISOString(),
        hasNewData: gtplIdChanged,
        idChanged: gtplIdChanged,
      };
    }

    // Process KABO machine data
    let kaboResponse = {
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
      machineType: "KABO_MACHINE_SMART200",
      priority: "medium",
      responseType: "kabo_machine",
      lastUpdate: null,
      recordId: 0,
      hasNewData: false,
      noNewData: true,
    };

    if (kaboData && kaboData.length > 0) {
      const kaboRecord = kaboData[0];
      // Use the actual column names from your database
      const kaboTimestamp =
        kaboRecord.created_on ||
        kaboRecord.timestamp ||
        kaboRecord.updated_at ||
        kaboRecord.date_created ||
        currentTime;

      // Check if ID has changed (new data)
      const kaboIdChanged = hasIdChanged(kaboRecord.id, lastKaboId);

      kaboResponse = {
        ...getMachineSpecificResponse(
          "kabo",
          kaboTimestamp,
          currentTime,
          kaboIdChanged
        ),
        recordId: kaboRecord.id,
        lastUpdate: new Date(kaboTimestamp).toISOString(),
        hasNewData: kaboIdChanged,
        idChanged: kaboIdChanged,
      };
    }

    // Prepare response with different JSON structure for each machine
    const response = {
      success: true,
      message:
        "Machine status retrieved successfully based on timestamp and ID changes",
      data: {
        gtpl: gtplResponse,
        kabo: kaboResponse,
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
      data: {
        gtpl: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "GTPL_122_S7_1200_01",
          priority: "high",
          responseType: "gtpl_machine",
          error: true,
        },
        kabo: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "KABO_MACHINE_SMART200",
          priority: "medium",
          responseType: "kabo_machine",
          error: true,
        },
      },
    });
  }
});

// Alternative endpoint without authentication for monitoring purposes
router.get("/status/public", async (req, res) => {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table with safer ordering
    const [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table with safer ordering
    const [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    const currentTime = new Date();

    // Process GTPL machine data
    let gtplResponse = {
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
      machineType: "GTPL_122_S7_1200_01",
      priority: "high",
      responseType: "gtpl_machine",
      lastUpdate: null,
      recordId: 0,
      hasNewData: false,
      noNewData: true,
    };

    if (gtplData && gtplData.length > 0) {
      const gtplRecord = gtplData[0];
      // Use the actual column names from your database
      const gtplTimestamp =
        gtplRecord.created_on ||
        gtplRecord.timestamp ||
        gtplRecord.updated_at ||
        gtplRecord.date_created ||
        currentTime;

      // Check if ID has changed (new data)
      const gtplIdChanged = hasIdChanged(gtplRecord.id, lastGtplId);

      gtplResponse = {
        ...getMachineSpecificResponse(
          "gtpl",
          gtplTimestamp,
          currentTime,
          gtplIdChanged
        ),
        recordId: gtplRecord.id,
        lastUpdate: new Date(gtplTimestamp).toISOString(),
        hasNewData: gtplIdChanged,
        idChanged: gtplIdChanged,
      };
    }

    // Process KABO machine data
    let kaboResponse = {
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
      machineType: "KABO_MACHINE_SMART200",
      priority: "medium",
      responseType: "kabo_machine",
      lastUpdate: null,
      recordId: 0,
      hasNewData: false,
      noNewData: true,
    };

    if (kaboData && kaboData.length > 0) {
      const kaboRecord = kaboData[0];
      // Use the actual column names from your database
      const kaboTimestamp =
        kaboRecord.created_on ||
        kaboRecord.timestamp ||
        kaboRecord.updated_at ||
        kaboRecord.date_created ||
        currentTime;

      // Check if ID has changed (new data)
      const kaboIdChanged = hasIdChanged(kaboRecord.id, lastKaboId);

      kaboResponse = {
        ...getMachineSpecificResponse(
          "kabo",
          kaboTimestamp,
          currentTime,
          kaboIdChanged
        ),
        recordId: kaboRecord.id,
        lastUpdate: new Date(kaboTimestamp).toISOString(),
        hasNewData: kaboIdChanged,
        idChanged: kaboIdChanged,
      };
    }

    // Prepare response with different JSON structure for each machine
    const response = {
      success: true,
      message:
        "Machine status retrieved successfully based on timestamp and ID changes",
      data: {
        gtpl: gtplResponse,
        kabo: kaboResponse,
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
      data: {
        gtpl: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "GTPL_122_S7_1200_01",
          priority: "high",
          responseType: "gtpl_machine",
          error: true,
        },
        kabo: {
          machineStatus: false,
          coolingStatus: false,
          internetStatus: false,
          machineType: "KABO_MACHINE_SMART200",
          priority: "medium",
          responseType: "kabo_machine",
          error: true,
        },
      },
    });
  }
});

// Test endpoint to see raw data from both tables
router.get("/test", async (req, res) => {
  try {
    // Get latest record from gtpl_122_s7_1200_01 table with safer ordering
    const [gtplData] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1"
    );

    // Get latest record from kabomachinedatasmart200 table with safer ordering
    const [kaboData] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1"
    );

    res.json({
      success: true,
      message: "Raw data from both tables (ordered by ID)",
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

// Diagnostic endpoint to check table structure
router.get("/diagnose", async (req, res) => {
  try {
    // Get table structure information
    const [gtplColumns] = await pool.query("DESCRIBE gtpl_122_s7_1200_01");

    const [kaboColumns] = await pool.query("DESCRIBE kabomachinedatasmart200");

    // Get sample data to see actual column names
    const [gtplSample] = await pool.query(
      "SELECT * FROM gtpl_122_s7_1200_01 LIMIT 1"
    );

    const [kaboSample] = await pool.query(
      "SELECT * FROM kabomachinedatasmart200 LIMIT 1"
    );

    res.json({
      success: true,
      message: "Table structure diagnosis",
      gtpl: {
        columns: gtplColumns,
        sampleData: gtplSample.length > 0 ? gtplSample[0] : null,
        availableColumns:
          gtplSample.length > 0 ? Object.keys(gtplSample[0]) : [],
      },
      kabo: {
        columns: kaboColumns,
        sampleData: kaboSample.length > 0 ? kaboSample[0] : null,
        availableColumns:
          kaboSample.length > 0 ? Object.keys(kaboSample[0]) : [],
      },
      timestamp: new Date().toISOString(),
    });
  } catch (error) {
    console.error("Error diagnosing tables:", error);
    res.status(500).json({
      success: false,
      message: "Error diagnosing tables",
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

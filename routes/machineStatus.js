const express = require("express");
const router = express.Router();
const { pool } = require("../db");
const { authenticateToken } = require("../middleware/auth");
const WebSocket = require("ws");
const { MACHINE_CONFIG } = require("../utils/machineConfig");

// Build MACHINE_TABLES from shared config (table -> display name)
const MACHINE_TABLES = {};
for (const [key, cfg] of Object.entries(MACHINE_CONFIG)) {
  if (!MACHINE_TABLES[cfg.table]) {
    // Extract short name from the key, e.g. "GTPL-122-gT-1000T-S7-1200" -> "GTPL_122"
    const shortName = key.split("-").slice(0, 2).join("_").replace(/-/g, "_");
    MACHINE_TABLES[cfg.table] = shortName;
  }
}

// Store last known timestamps and IDs for comparison
const previousState = {};
Object.keys(MACHINE_TABLES).forEach((table) => {
  previousState[table] = { id: null, timestamp: null };
});

let lastUpdateTime = null;
let timeoutInterval = null;

// Legacy aliases for backward compatibility
let lastGtplTimestamp = null;
let lastKaboTimestamp = null;
let lastGtplId = null;
let lastKaboId = null;

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

// Function to check and broadcast machine status for ALL machines
async function checkAndBroadcastMachineStatus(wss) {
  try {
    const currentTime = new Date();
    let hasAnyUpdate = false;
    const machines = [];

    for (const [tableName, machineName] of Object.entries(MACHINE_TABLES)) {
      try {
        const [rows] = await pool.query(
          `SELECT * FROM \`${tableName}\` ORDER BY id DESC LIMIT 1`
        );
        const record = rows[0];
        if (!record) continue;

        const id = record.id || 0;
        const timestamp = record.created_at || record.created_on || currentTime;
        const isoTime = new Date(timestamp).toISOString();

        const prev = previousState[tableName];
        const normalizedTimestamp = new Date(timestamp).getTime();
        const normalizedPrev = prev.timestamp ? new Date(prev.timestamp).getTime() : null;

        const idChanged = prev.id !== null ? id > prev.id : false;
        const timeChanged = normalizedPrev !== null ? normalizedTimestamp !== normalizedPrev : false;
        const hasNewData = idChanged && timeChanged;

        if (idChanged || timeChanged) {
          hasAnyUpdate = true;
        }

        previousState[tableName] = { id, timestamp };

        machines.push({
          ...getMachineSpecificResponse(machineName, timestamp, currentTime, hasNewData),
          recordId: id,
          lastUpdate: isoTime,
          hasNewData,
          idChanged,
          machineName,
          tableName,
          createdAtChanged: timeChanged,
          createdOnChanged: timeChanged,
        });
      } catch (tableErr) {
        // Skip tables that don't exist or have errors
        console.error(`Error querying ${tableName}:`, tableErr.message);
      }
    }

    // Also maintain legacy gtpl/kabo keys for backward compatibility
    const gtplMachine = machines.find(m => m.tableName === "gtpl_122_s7_1200_01");
    const kaboMachine = machines.find(m => m.tableName === "kabomachinedatasmart200");

    if (hasAnyUpdate || machines.length > 0) {
      lastUpdateTime = currentTime;
      startTimeoutReset(wss);

      const response = {
        success: true,
        message: "Machine status updated based on timestamp and ID changes",
        data: {
          gtpl: gtplMachine || {
            machineStatus: false, coolingStatus: false, internetStatus: false,
            machineType: "GTPL_122_S7_1200_01", priority: "high",
            responseType: "gtpl_machine", noNewData: true,
          },
          kabo: kaboMachine || {
            machineStatus: false, coolingStatus: false, internetStatus: false,
            machineType: "KABO_MACHINE_SMART200", priority: "medium",
            responseType: "kabo_machine", noNewData: true,
          },
          machines,
        },
        timestamp: currentTime.toISOString(),
      };

      broadcastData(wss, {
        type: "machine_status_update",
        data: response,
        timestamp: currentTime.toISOString(),
      });
    }
  } catch (error) {
    console.error("Error checking machine status:", error);

    broadcastData(wss, {
      type: "machine_status_error",
      data: {
        success: false,
        message: "Error checking machine status",
        error: process.env.NODE_ENV === "development" ? error.message : "Internal server error",
      },
      timestamp: new Date().toISOString(),
    });
  }
}

// Helper to fetch status for all machines
async function getAllMachineStatuses() {
  const currentTime = new Date();
  const machines = [];

  for (const [tableName, machineName] of Object.entries(MACHINE_TABLES)) {
    try {
      const [rows] = await pool.query(
        `SELECT * FROM \`${tableName}\` ORDER BY id DESC LIMIT 1`
      );
      const record = rows[0];
      if (!record) continue;

      const id = record.id || 0;
      const timestamp = record.created_at || record.created_on || currentTime;
      const isoTime = new Date(timestamp).toISOString();

      const prev = previousState[tableName];
      const normalizedTimestamp = new Date(timestamp).getTime();
      const normalizedPrev = prev.timestamp ? new Date(prev.timestamp).getTime() : null;

      const idChanged = prev.id !== null ? id > prev.id : false;
      const timeChanged = normalizedPrev !== null ? normalizedTimestamp !== normalizedPrev : false;
      const hasNewData = idChanged && timeChanged;

      machines.push({
        ...getMachineSpecificResponse(machineName, timestamp, currentTime, hasNewData),
        recordId: id,
        lastUpdate: isoTime,
        hasNewData,
        idChanged,
        machineName,
        tableName,
        createdAtChanged: timeChanged,
        createdOnChanged: timeChanged,
      });
    } catch (tableErr) {
      // Skip tables that error
    }
  }

  // Legacy gtpl/kabo keys for backward compatibility
  const gtplMachine = machines.find(m => m.tableName === "gtpl_122_s7_1200_01") || {
    machineStatus: false, coolingStatus: false, internetStatus: false,
    machineType: "GTPL_122_S7_1200_01", priority: "high",
    responseType: "gtpl_machine", noNewData: true,
  };
  const kaboMachine = machines.find(m => m.tableName === "kabomachinedatasmart200") || {
    machineStatus: false, coolingStatus: false, internetStatus: false,
    machineType: "KABO_MACHINE_SMART200", priority: "medium",
    responseType: "kabo_machine", noNewData: true,
  };

  return { machines, gtpl: gtplMachine, kabo: kaboMachine, currentTime };
}

// Machine Status API
router.get("/status", async (req, res) => {
  try {
    const { machines, gtpl, kabo, currentTime } = await getAllMachineStatuses();

    res.json({
      success: true,
      message: "Machine status retrieved successfully based on timestamp and ID changes",
      data: {
        gtpl,
        kabo,
        machines,
      },
      timestamp: currentTime.toISOString(),
    });
  } catch (error) {
    console.error("Error fetching machine status:", error);
    res.status(500).json({
      success: false,
      message: "Error fetching machine status",
      error: process.env.NODE_ENV === "development" ? error.message : "Internal server error",
    });
  }
});

// Alternative endpoint without authentication for monitoring purposes
router.get("/status/public", async (req, res) => {
  try {
    const { machines, gtpl, kabo, currentTime } = await getAllMachineStatuses();

    res.json({
      success: true,
      message: "Machine status retrieved successfully based on timestamp and ID changes",
      data: {
        gtpl,
        kabo,
        machines,
      },
      timestamp: currentTime.toISOString(),
    });
  } catch (error) {
    console.error("Error fetching machine status:", error);
    res.status(500).json({
      success: false,
      message: "Error fetching machine status",
      error: process.env.NODE_ENV === "development" ? error.message : "Internal server error",
    });
  }
});

// Test endpoint to see raw data from all tables
router.get("/test", async (req, res) => {
  try {
    const table = req.query.table;
    const results = {};

    if (table) {
      // Query specific table
      if (!MACHINE_TABLES[table]) {
        return res.status(400).json({ success: false, error: "Invalid table name" });
      }
      const [rows] = await pool.query(`SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT 1`);
      results[table] = rows.length > 0 ? rows[0] : null;
    } else {
      // Legacy: query gtpl and kabo
      const [gtplData] = await pool.query("SELECT * FROM gtpl_122_s7_1200_01 ORDER BY id DESC LIMIT 1");
      const [kaboData] = await pool.query("SELECT * FROM kabomachinedatasmart200 ORDER BY id DESC LIMIT 1");
      results.gtplData = gtplData.length > 0 ? gtplData[0] : null;
      results.kaboData = kaboData.length > 0 ? kaboData[0] : null;
    }

    res.json({
      success: true,
      message: "Raw data (ordered by ID)",
      ...results,
      availableTables: Object.keys(MACHINE_TABLES),
      timestamp: new Date().toISOString(),
    });
  } catch (error) {
    console.error("Error fetching test data:", error);
    res.status(500).json({
      success: false,
      message: "Error fetching test data",
      error: process.env.NODE_ENV === "development" ? error.message : "Internal server error",
    });
  }
});

// Diagnostic endpoint to check table structure
router.get("/diagnose", async (req, res) => {
  try {
    const table = req.query.table;
    const diagnosis = {};

    const tablesToCheck = table ? [table] : ["gtpl_122_s7_1200_01", "kabomachinedatasmart200"];

    for (const t of tablesToCheck) {
      try {
        const [columns] = await pool.query(`DESCRIBE \`${t}\``);
        const [sample] = await pool.query(`SELECT * FROM \`${t}\` LIMIT 1`);
        diagnosis[t] = {
          columns,
          sampleData: sample.length > 0 ? sample[0] : null,
          availableColumns: sample.length > 0 ? Object.keys(sample[0]) : [],
        };
      } catch (tableErr) {
        diagnosis[t] = { error: tableErr.message };
      }
    }

    res.json({
      success: true,
      message: "Table structure diagnosis",
      ...diagnosis,
      availableTables: Object.keys(MACHINE_TABLES),
      timestamp: new Date().toISOString(),
    });
  } catch (error) {
    console.error("Error diagnosing tables:", error);
    res.status(500).json({
      success: false,
      message: "Error diagnosing tables",
      error: process.env.NODE_ENV === "development" ? error.message : "Internal server error",
    });
  }
});

module.exports = {
  router,
  checkAndBroadcastMachineStatus,
  startTimeoutReset,
};

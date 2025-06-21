const express = require("express");
const router = express.Router();
const { pool } = require("../db");
const { authenticateToken } = require("../middleware/auth");

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

    // Determine cooling status (assuming there's a cooling-related field in the data)
    // You may need to adjust this based on your actual data structure
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

module.exports = router;

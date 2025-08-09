const express = require("express");
const router = express.Router();
const { pool } = require("../db");

// Helper function similar to machineStatus.getMachineSpecificResponse
function getMachineSpecificResponse(machineType, timestamp, currentTime, hasNewData) {
  const timestampDate = new Date(timestamp);
  const fiveMinutesAgo = new Date(currentTime.getTime() - 5 * 60 * 1000);
  const oneMinuteAgo = new Date(currentTime.getTime() - 1 * 60 * 1000);
  const thirtySecondsAgo = new Date(currentTime.getTime() - 30 * 1000);

  if (!hasNewData) {
    return {
      machineStatus: false,
      coolingStatus: false,
      internetStatus: false,
      machineType: machineType.startsWith("GTPL") ? machineType : machineType,
      priority: machineType.startsWith("GTPL") ? "high" : "medium",
      responseType: machineType.startsWith("GTPL") ? "gtpl_machine" : "kabo_machine",
      noNewData: true,
    };
  }

  return {
    machineStatus: timestampDate > fiveMinutesAgo,
    coolingStatus: timestampDate > oneMinuteAgo,
    internetStatus: timestampDate > thirtySecondsAgo,
    machineType,
    priority: machineType.startsWith("GTPL") ? "high" : "medium",
    responseType: machineType.startsWith("GTPL") ? "gtpl_machine" : "kabo_machine",
    noNewData: false,
  };
}

const MACHINE_TABLES = {
  gtpl_122_s7_1200_01: "GTPL_122_S7_1200",
  kabomachinedatasmart200: "KABO_200",
  GTPL_108_gT_40E_P_S7_200_Germany: "GTPL_108",
  GTPL_109_gT_40E_P_S7_200_Germany: "GTPL_109",
  GTPL_110_gT_40E_P_S7_200_Germany: "GTPL_110",
  GTPL_111_gT_80E_P_S7_200_Germany: "GTPL_111",
  GTPL_112_gT_80E_P_S7_200_Germany: "GTPL_112",
  GTPL_113_gT_80E_P_S7_200_Germany: "GTPL_113",
  GTPL_114_GT_140E_S7_1200: "GTPL_114",
  GTPL_115_GT_180E_S7_1200: "GTPL_115",
  GTPL_119_GT_180E_S7_1200: "GTPL_119",
  GTPL_120_GT_180E_S7_1200: "GTPL_120",
  GTPL_116_GT_240E_S7_1200: "GTPL_116",
  GTPL_117_GT_320E_S7_1200: "GTPL_117",
  GTPL_121_GT1000T: "GTPL_121",
};

const previousState = {};
Object.keys(MACHINE_TABLES).forEach(table => {
  previousState[table] = { id: null, timestamp: null };
});

router.get("/", async (req, res) => {
  const currentTime = new Date();
  const machines = [];

  try {
    for (const [tableName, machineName] of Object.entries(MACHINE_TABLES)) {
      const [rows] = await pool.query(`SELECT * FROM ${tableName} ORDER BY id DESC LIMIT 1`);
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
      previousState[tableName] = { id, timestamp };

      const baseResponse = {
        ...getMachineSpecificResponse(machineName, timestamp, currentTime, hasNewData),
        recordId: id,
        lastUpdate: isoTime,
        hasNewData,
        idChanged,
        machineName,
        createdAtChanged: timeChanged,
        createdOnChanged: timeChanged,
      };

      const rawCondFan = record.COND_FAN_ON ?? record.cond_fan_on;
      if (rawCondFan !== undefined) {
        const condFanOn = rawCondFan === 1 || rawCondFan === true || rawCondFan === "tr" || rawCondFan === "true";
        baseResponse.condFanOn = condFanOn;
      }

      machines.push(baseResponse);
    }

    res.json({
      success: true,
      message: "Machine statuses retrieved successfully",
      data: machines,
      timestamp: currentTime.toISOString(),
    });
  } catch (err) {
    console.error("\u274c Error fetching machine statuses:", err);
    res.status(500).json({
      success: false,
      message: "Error fetching machine statuses",
      error: process.env.NODE_ENV === "development" ? err.message : "Internal server error",
    });
  }
});

module.exports = router;
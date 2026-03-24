const express = require("express");
const router = express.Router();
const { pool } = require("../db");
const { MACHINE_CONFIG, MACHINE_NAME_ALIASES } = require("../utils/machineConfig");

// ------------------ Fault Check Utility ------------------
async function checkFaultsAndNotify(record, machineName) {
  const machineConfig = MACHINE_CONFIG[machineName];
  if (!machineConfig) return [];

  const activeFaults = [];
  for (const tag of machineConfig.tags) {
    const value = record[tag];
    if (
      value === true ||
      value === 1 ||
      value === "tr" ||
      (typeof value === "string" && value.toLowerCase() === "true")
    ) {
      activeFaults.push({ tag, value, machineType: machineConfig.type });
    }
  }

  if (activeFaults.length > 0) {
    try {
      await fetch(process.env.FAULT_API_ENDPOINT || "YOUR_API_ENDPOINT_HERE", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          recordId: record.id,
          machineName,
          machineType: machineConfig.type,
          table: machineConfig.table,
          timestamp: new Date().toISOString(),
          activeFaults,
          recordData: record,
        }),
      });
    } catch (err) {
      console.error("API push failed:", err);
    }
  }

  return activeFaults;
}

async function getPreviousRecords(currentRecordId, machineName, table, limit = 10) {
  try {
    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` WHERE id < ? ORDER BY id DESC LIMIT ?`,
      [currentRecordId, limit]
    );
    return rows || [];
  } catch (error) {
    console.error("DB fetch error:", error);
    return [];
  }
}

// ------------------ GET Handler ------------------
router.get("/", async (req, res) => {
  const page = parseInt(req.query.page || "1", 10);
  const limit = parseInt(req.query.limit || "10", 10);
  const offset = (page - 1) * limit;

  const rawMachineName = (req.query.machineName || "").trim();
  const machineName = MACHINE_NAME_ALIASES[rawMachineName] || rawMachineName;
  const checkFaults = req.query.checkFaults === "true";
  const search = (req.query.search || "").trim();

  const machineConfig = MACHINE_CONFIG[machineName];
  if (!machineConfig) {
    return res.status(400).json({
      error: `No configuration found for machine: ${machineName}`,
      availableMachines: Object.keys(MACHINE_CONFIG),
    });
  }

  try {
    const table = machineConfig.table;
    const values = [];
    let whereClause = "";

    if (search) {
      whereClause = "WHERE machine_name LIKE ?";
      values.push(`%${search}%`);
    }

    const [countRows] = await pool.query(
      `SELECT COUNT(*) AS count FROM \`${table}\` ${whereClause}`,
      values
    );
    const total = countRows[0] ? Number(countRows[0].count) : 0;

    const [result] = await pool.query(
      `SELECT * FROM \`${table}\` ${whereClause} ORDER BY id DESC LIMIT ? OFFSET ?`,
      [...values, limit, offset]
    );

    const rows = Array.isArray(result) ? result : [];
    const totalPages = Math.max(1, Math.ceil(total / limit));

    if (rows.length === 0) {
      return res.json({
        data: [],
        total: 0,
        totalPages: 1,
        page,
        limit,
        message: `No records found for machine: ${machineName}`,
        machineName,
        machineType: machineConfig.type,
        table,
        faultCheckingEnabled: checkFaults,
      });
    }

    const enhancedData = [];
    for (const record of rows) {
      let faultInfo = null;
      let previousRecords = null;

      if (checkFaults) {
        const activeFaults = await checkFaultsAndNotify(record, machineName);
        if (activeFaults.length > 0) {
          previousRecords = await getPreviousRecords(record.id, machineName, table);
        }
        faultInfo = {
          activeFaults,
          faultCount: activeFaults.length,
          machineType: machineConfig.type,
          previousRecords,
        };
      }

      enhancedData.push({
        ...record,
        faultInfo,
      });
    }

    return res.json({
      data: enhancedData,
      total,
      totalPages,
      page,
      limit,
      search,
      machineName,
      machineType: machineConfig.type,
      table,
      faultCheckingEnabled: checkFaults,
    });
  } catch (err) {
    console.error("Unhandled error:", err.message || err);
    res.status(500).json({ error: "Internal server error" });
  }
});

module.exports = router;
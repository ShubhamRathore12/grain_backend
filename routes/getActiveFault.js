const express = require('express');
const router = express.Router();
const { pool } = require('../db');
const { MACHINE_NAME_ALIASES, MACHINE_CONFIG } = require('../utils/machineConfig');


router.get('/', async (req, res) => {
  try {
    // Resolve machine name (with alias map)
    const rawMachineName = req.query.machineName?.trim() || "";
    const machineName = MACHINE_NAME_ALIASES[rawMachineName] || rawMachineName;

    // Optional: override table via query
    const overrideTable = req.query.tableName?.trim();

    // Pagination params - ensure proper parsing
    const pageParam = req.query.page;
    const limitParam = req.query.limit;

    const page = pageParam ? Math.max(1, parseInt(pageParam, 10)) : 1;
    const limit = limitParam
      ? Math.min(Math.max(1, parseInt(limitParam, 10)), 2000)
      : 200;

    // Ensure page and limit are valid numbers
    if (!Number.isInteger(page) || !Number.isInteger(limit)) {
      return res.status(400).json({
        error: "Invalid pagination parameters",
        page: pageParam,
        limit: limitParam,
      });
    }

    const offset = (page - 1) * limit;

    const machineConfig =
      MACHINE_CONFIG[machineName];

    if (!machineConfig) {
      return res.status(400).json({
        error: `No configuration found for machine: ${machineName}`,
        availableMachines: Object.keys(MACHINE_CONFIG),
      });
    }

    const table = overrideTable || machineConfig.table;

    console.log(
      `Fetching data for machine: ${machineName}, table: ${table}, page: ${page}, limit: ${limit}, offset: ${offset}`
    );

    // 1) Total count
    const [countRows] = await pool.query(
      `SELECT COUNT(*) AS cnt FROM \`${table}\``
    );
    const total = countRows?.[0]?.cnt ?? 0;
    const totalPages = Math.max(1, Math.ceil(total / limit));

    console.log(`Total records: ${total}, Total pages: ${totalPages}`);

    // Validate page number against total pages
    if (page > totalPages && total > 0) {
      return res.status(400).json({
        error: `Page ${page} does not exist. Maximum page is ${totalPages}`,
        total,
        totalPages,
        requestedPage: page,
      });
    }

    // 2) Page data ordered by newest first to enable correct pagination across history
    const [rows] = await pool.query(
      `SELECT * FROM \`${table}\` ORDER BY id DESC LIMIT ? OFFSET ?`,
      [limit, offset]
    );

    const data = Array.isArray(rows) ? rows : [];
    console.log(`Returned ${data.length} records for page ${page}`);

    return res.json({
      data,
      total,
      totalPages,
      page,
      limit,
      offset,
      machineName,
      machineType: machineConfig.type,
      hasNextPage: page < totalPages,
      hasPreviousPage: page > 1,
    });
  } catch (err) {
    console.error("Error fetching logs:", err?.message || err);
    return res.status(500).json({
      error: "Failed to fetch logs",
      details: err?.message,
    });
  }
});

module.exports = router;
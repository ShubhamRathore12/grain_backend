const { pool } = require("../db");

// Middleware to check database connection
async function checkDatabaseConnection(req, res, next) {
  try {
    // Test database connection
    const connection = await pool.getConnection();
    connection.release();
    next();
  } catch (error) {
    console.error("Database connection error in middleware:", error.message);
    res.status(503).json({
      error: "Database service unavailable",
      message:
        "The database is currently not accessible. Please try again later.",
      details:
        process.env.NODE_ENV === "development" ? error.message : undefined,
    });
  }
}

module.exports = checkDatabaseConnection;

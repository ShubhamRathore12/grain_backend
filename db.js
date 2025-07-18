const mysql = require("mysql2/promise");
require("dotenv").config();

// Track database connection status
let isDatabaseConnected = false;
let lastConnectionAttempt = 0;
const CONNECTION_RETRY_INTERVAL = 30000; // 30 seconds

const pool = mysql.createPool({
  host: process.env.DB_HOST,
  port: process.env.DB_PORT,
  user: process.env.DB_USER,
  password: process.env.DB_PASSWORD,
  database: process.env.DB_NAME,
  waitForConnections: true,
  connectionLimit: 10,
  queueLimit: 0,
  enableKeepAlive: true,
  keepAliveInitialDelay: 0,
  connectTimeout: 30000, // Increased timeout for deployment
  acquireTimeout: 30000, // Added acquire timeout
  timeout: 30000, // Added query timeout
  dateStrings: true,
  ssl:
    process.env.NODE_ENV === "production"
      ? { rejectUnauthorized: false }
      : false,
});

// Test database connection
async function testConnection() {
  try {
    console.log(
      `Attempting to connect to database: ${process.env.DB_HOST}:${process.env.DB_PORT}/${process.env.DB_NAME}`
    );
    const connection = await pool.getConnection();
    console.log("Database connection successful");
    connection.release();
    isDatabaseConnected = true;
    return true;
  } catch (error) {
    console.error("Database connection failed:", error.message);
    console.error("Connection details:", {
      host: process.env.DB_HOST,
      port: process.env.DB_PORT,
      database: process.env.DB_NAME,
      user: process.env.DB_USER,
      hasPassword: !!process.env.DB_PASSWORD,
    });
    isDatabaseConnected = false;
    return false;
  }
}

// Safe database query function with connection check
async function safeQuery(query, params = []) {
  const now = Date.now();

  // If we recently failed to connect, don't try again immediately
  if (
    !isDatabaseConnected &&
    now - lastConnectionAttempt < CONNECTION_RETRY_INTERVAL
  ) {
    throw new Error("Database connection unavailable");
  }

  lastConnectionAttempt = now;

  try {
    const [rows] = await pool.query(query, params);
    isDatabaseConnected = true;
    return rows;
  } catch (error) {
    isDatabaseConnected = false;
    console.error("Database query error:", error.message);
    throw error;
  }
}

// Function to ensure the users table exists
async function ensureUserTableExists() {
  try {
    await safeQuery(`
      CREATE TABLE IF NOT EXISTS kabu_users (
        id INT AUTO_INCREMENT PRIMARY KEY,
        accountType VARCHAR(50),
        firstName VARCHAR(100),
        lastName VARCHAR(100),
        username VARCHAR(100) UNIQUE,
        email VARCHAR(100),
        phoneNumber VARCHAR(20),
        company VARCHAR(100),
        password VARCHAR(255),
        monitorAccess VARCHAR(50),
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
      )
    `);
    console.log("Users table checked/created successfully");
  } catch (error) {
    console.error("Error creating users table:", error);
    throw error;
  }
}

// Initialize database
async function initializeDatabase() {
  const maxRetries = 3;
  let retryCount = 0;

  while (retryCount < maxRetries) {
    try {
      console.log(
        `Attempting database connection (attempt ${
          retryCount + 1
        }/${maxRetries})...`
      );
      const isConnected = await testConnection();
      if (!isConnected) {
        throw new Error("Database connection test failed");
      }
      await ensureUserTableExists();
      console.log("Database initialization completed successfully");
      return;
    } catch (error) {
      retryCount++;
      console.error(
        `Database connection attempt ${retryCount} failed:`,
        error.message
      );

      if (retryCount >= maxRetries) {
        console.error("All database connection attempts failed");
        throw new Error(
          "Failed to connect to database after multiple attempts"
        );
      }

      // Wait before retrying (exponential backoff)
      const waitTime = Math.pow(2, retryCount) * 1000;
      console.log(`Waiting ${waitTime}ms before retry...`);
      await new Promise((resolve) => setTimeout(resolve, waitTime));
    }
  }
}

module.exports = {
  pool,
  safeQuery,
  ensureUserTableExists,
  initializeDatabase,
  testConnection,
  isDatabaseConnected: () => isDatabaseConnected,
};

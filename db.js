const mysql = require("mysql2/promise");
require("dotenv").config();

const pool = mysql.createPool({
  host: process.env.DB_HOST,
  port: process.env.DB_PORT,
  user: process.env.DB_USER,
  password: process.env.DB_PASSWORD,
  database: process.env.DB_NAME,
  waitForConnections: true,
  connectionLimit: 10,
  queueLimit: 0,
});

// Function to ensure the users table exists
async function ensureUserTableExists() {
  try {
    await pool.query(`
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

module.exports = {
  pool,
  ensureUserTableExists,
};

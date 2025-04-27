const express = require("express");
const router = express.Router();
const { pool, ensureUserTableExists } = require("../db");

// Registration route
router.post("/", async (req, res) => {
  const {
    accountType,
    firstName,
    lastName,
    username,
    email,
    phoneNumber,
    company,
    password,
    monitorAccess,
  } = req.body;

  try {
    await ensureUserTableExists();

    // Check for duplicate username
    const [existingUser] = await pool.query(
      "SELECT * FROM kabu_users WHERE username = ?",
      [username]
    );

    if (Array.isArray(existingUser) && existingUser.length > 0) {
      return res.status(409).json({
        message: "Username already exists. Please choose another.",
      });
    }

    // Convert monitorAccess array to string if it's an array
    const monitorAccessStr = Array.isArray(monitorAccess)
      ? monitorAccess.join(",")
      : monitorAccess || "";

    // Insert new user
    await pool.query(
      `INSERT INTO kabu_users 
            (accountType, firstName, lastName, username, email, phoneNumber, company, password, monitorAccess) 
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      [
        accountType,
        firstName,
        lastName,
        username,
        email || null,
        phoneNumber,
        company,
        password,
        monitorAccessStr,
      ]
    );

    res.status(201).json({
      success: true,
      message: "User registered successfully",
    });
  } catch (error) {
    console.error("Registration error:", error);
    res.status(500).json({
      success: false,
      message: "Server error occurred during registration",
      error: error.message,
    });
  }
});

module.exports = router;

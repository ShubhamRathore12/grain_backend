const express = require("express");
const router = express.Router();
const jwt = require("jsonwebtoken");
const { pool, ensureUserTableExists } = require("../db");

// Login route
router.post("/login", async (req, res) => {
  const { username, password } = req.body;

  try {
    const [rows] = await pool.query(
      "SELECT * FROM kabu_users WHERE username = ? AND password = ?",
      [username, password]
    );

    if (!rows || rows.length === 0) {
      return res.status(401).json({ message: "Invalid username or password" });
    }

    const user = rows[0];
    const token = jwt.sign(
      {
        username: user.username,
        accountType: user.accountType,
        userId: user.id,
      },
      process.env.JWT_SECRET,
      { expiresIn: "15m" }
    );

    // Remove password from response
    const { password: _, ...userWithoutPassword } = user;

    res.cookie("auth_token", token, {
      httpOnly: true,
      sameSite: "lax",
      maxAge: 15 * 60 * 1000, // 15 minutes
      path: "/dashboard",
    });

    res.json({
      message: "Login successful",
      user: userWithoutPassword,
      token: token,
    });
  } catch (error) {
    console.error("Login error:", error);
    res.status(500).json({ message: "Server error while logging in" });
  }
});

// Registration route
router.post("/register", async (req, res) => {
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
    location,
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
      (accountType, firstName, lastName, username, email, phoneNumber, company, password, monitorAccess,location) 
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?,?)`,
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
        location || null
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

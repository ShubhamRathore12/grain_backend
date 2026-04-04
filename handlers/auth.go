package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"grain_backend/database"
	"grain_backend/middleware"
	"grain_backend/models"
)

// HandleLogin handles user login
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var loginReq models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Query database
	query := "SELECT id, accountType, firstName, lastName, username, email, phoneNumber, company, password, monitorAccess, location, created_at FROM kabu_users WHERE username = ? AND password = ?"
	rows, err := database.SafeQuery(query, loginReq.Username, loginReq.Password)
	if err != nil {
		log.Printf("Login query error: %v", err)
		
		if strings.Contains(err.Error(), "Database connection unavailable") ||
			strings.Contains(err.Error(), "ETIMEDOUT") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"message": "Database service temporarily unavailable. Please try again later.",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Server error while logging in",
		})
		return
	}
	defer rows.Close()

	// Check if user exists
	if !rows.Next() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Invalid username or password",
		})
		return
	}

	// Scan user data (using sql.NullString for nullable columns)
	var user models.User
	var createdAt time.Time
	var accountType, firstName, lastName, email, phoneNumber, company, password, monitorAccess, location sql.NullString
	err = rows.Scan(
		&user.ID, &accountType, &firstName, &lastName,
		&user.Username, &email, &phoneNumber, &company,
		&password, &monitorAccess, &location, &createdAt,
	)
	if err == nil {
		user.AccountType = accountType.String
		user.FirstName = firstName.String
		user.LastName = lastName.String
		user.Email = email.String
		user.PhoneNumber = phoneNumber.String
		user.Company = company.String
		user.Password = password.String
		user.MonitorAccess = monitorAccess.String
		user.Location = location.String
		user.CreatedAt = createdAt
	}
	if err != nil {
		log.Printf("Error scanning user: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Server error",
		})
		return
	}
	rows.Close()

	// Generate JWT token
	token, err := middleware.GenerateToken(user.Username, user.AccountType, user.ID)
	if err != nil {
		log.Printf("Token generation error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Error generating token",
		})
		return
	}

	// Set cookie
	middleware.SetAuthCookie(w, token)

	// Return success response
	response := models.LoginResponse{
		Message: "Login successful",
		User:    &user,
		Token:   token,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleRegister handles user registration
func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var regReq models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&regReq); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Check if username already exists
	checkQuery := "SELECT id FROM kabu_users WHERE username = ?"
	rows, err := database.SafeQuery(checkQuery, regReq.Username)
	if err != nil {
		log.Printf("Username check error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Server error occurred during registration",
			"error":   err.Error(),
		})
		return
	}
	defer rows.Close()

	if rows.Next() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Username already exists. Please choose another.",
		})
		return
	}
	rows.Close()

	// Convert monitorAccess array to comma-separated string
	monitorAccessStr := ""
	if len(regReq.MonitorAccess) > 0 {
		monitorAccessStr = strings.Join(regReq.MonitorAccess, ",")
	}

	// Insert new user
	insertQuery := `INSERT INTO kabu_users 
		(accountType, firstName, lastName, username, email, phoneNumber, company, password, monitorAccess, location) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = database.SafeExec(insertQuery,
		regReq.AccountType,
		regReq.FirstName,
		regReq.LastName,
		regReq.Username,
		regReq.Email,
		regReq.PhoneNumber,
		regReq.Company,
		regReq.Password,
		monitorAccessStr,
		regReq.Location,
	)

	if err != nil {
		log.Printf("Registration error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Server error occurred during registration",
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "User registered successfully",
	})
}

package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"grain_backend/config"
	"grain_backend/database"
	"grain_backend/middleware"
	"grain_backend/models"
)

// Rate limits for the credential endpoints. Login is limited per IP and per
// username so neither a single host nor a single targeted account can be ground
// through a password list (S-03).
const (
	loginAttemptsPerIP       = 10
	loginAttemptsPerUsername = 5
	loginWindow              = 5 * time.Minute
	passwordChangeAttempts   = 5
	passwordChangeWindow     = 15 * time.Minute
)

// Accounts exempt from login rate limiting. These are shared operator logins
// that sign in from many hosts at once, so the per-IP and per-username caps
// lock them out during normal use. Keys must be lowercase.
var loginRateLimitExempt = map[string]bool{
	"yogendra":  true,
	"narayan12": true,
}

func isLoginRateLimitExempt(username string) bool {
	return loginRateLimitExempt[strings.ToLower(strings.TrimSpace(username))]
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// HandleLogin authenticates a user and issues a session cookie.
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"success": false, "message": "Method not allowed",
		})
		return
	}

	var loginReq models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "Invalid request body",
		})
		return
	}

	username := strings.TrimSpace(loginReq.Username)
	if username == "" || loginReq.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "Username and password are required",
		})
		return
	}

	clientIP := middleware.ClientIP(r)
	if !isLoginRateLimitExempt(username) {
		if ok, retryAfter := middleware.AllowRequest("login-ip", clientIP, loginAttemptsPerIP, loginWindow); !ok {
			log.Printf("auth: login rate limit hit for ip=%s", clientIP)
			middleware.WriteRateLimited(w, retryAfter)
			return
		}
		if ok, retryAfter := middleware.AllowRequest("login-user", strings.ToLower(username), loginAttemptsPerUsername, loginWindow); !ok {
			log.Printf("auth: login rate limit hit for username=%s ip=%s", username, clientIP)
			middleware.WriteRateLimited(w, retryAfter)
			return
		}
	}

	// Look the user up by username only. The old query matched username AND
	// password in SQL, which required the password to be stored in plaintext and
	// made hashing impossible.
	query := `SELECT id, accountType, firstName, lastName, username, email, phoneNumber,
	                 company, password, monitorAccess, location, created_at
	          FROM kabu_users WHERE username = ? LIMIT 1`

	var (
		user      models.User
		createdAt sql.NullTime
		stored    sql.NullString

		accountType, firstName, lastName    sql.NullString
		email, phoneNumber, company         sql.NullString
		monitorAccessCol, locationCol       sql.NullString
	)

	err := database.SafeQueryRowContext(r.Context(), query, username).Scan(
		&user.ID, &accountType, &firstName, &lastName,
		&user.Username, &email, &phoneNumber, &company,
		&stored, &monitorAccessCol, &locationCol, &createdAt,
	)

	switch {
	case err == sql.ErrNoRows:
		// Same wording and status as a wrong password: the response must not
		// confirm which usernames exist.
		log.Printf("auth: failed login for unknown username=%q ip=%s", username, clientIP)
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false, "message": "Invalid username or password",
		})
		return
	case err != nil:
		log.Printf("auth: login query error: %v", err)
		if strings.Contains(err.Error(), "Database connection unavailable") ||
			strings.Contains(err.Error(), "ETIMEDOUT") {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"success": false,
				"message": "Database service temporarily unavailable. Please try again later.",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Server error while logging in",
		})
		return
	}

	check := VerifyPassword(stored.String, loginReq.Password)
	if !check.Valid {
		log.Printf("auth: failed login for username=%q ip=%s", username, clientIP)
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false, "message": "Invalid username or password",
		})
		return
	}

	user.AccountType = accountType.String
	user.FirstName = firstName.String
	user.LastName = lastName.String
	user.Email = email.String
	user.PhoneNumber = phoneNumber.String
	user.Company = company.String
	user.MonitorAccess = monitorAccessCol.String
	user.Location = locationCol.String
	user.CreatedAt = createdAt.Time

	// A correct password found in the legacy plaintext column gets rewritten as a
	// hash immediately, so the cleartext value stops existing at rest.
	if check.Upgraded != "" {
		if _, upErr := database.SafeExecContext(r.Context(),
			"UPDATE kabu_users SET password = ? WHERE id = ?", check.Upgraded, user.ID); upErr != nil {
			log.Printf("auth: could not upgrade stored password for user %d: %v", user.ID, upErr)
		} else {
			log.Printf("auth: upgraded plaintext password to bcrypt for user %d", user.ID)
		}
	}

	issued, err := middleware.GenerateToken(user.Username, user.AccountType, user.ID)
	if err != nil {
		log.Printf("auth: token generation error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Error generating session",
		})
		return
	}

	middleware.SetAuthCookie(w, issued)

	// mustChangePassword tells the UI to route straight to the change-password
	// screen. The server does not yet block other calls in that state, because
	// doing so would lock out every existing account the moment this ships;
	// enable enforcement once accounts have rotated (see the remediation doc).
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":            true,
		"message":            "Login successful",
		"user":               &user,
		"token":              issued.Token,
		"expiresAt":          issued.ExpiresAt.UTC().Format(time.RFC3339),
		"mustChangePassword": check.MustRotate,
	})
}

// HandleLogout revokes the presented session server-side and clears the cookie.
//
// The reported defect was that logout only navigated back to the login page: the
// token stayed valid, so reopening a machine URL restored access (S-05). The fix
// has to happen here, not in the client.
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetUserFromContext(r)
	if !ok {
		// Never fail a logout. Clearing the cookie is still the right outcome.
		middleware.ClearAuthCookie(w)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true, "message": "Logged out",
		})
		return
	}

	// "all=true" kills every session for the account, for a lost device or a
	// suspected credential leak.
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all")), "true") {
		middleware.RevokeAllSessionsForUser(claims.UserID)
		log.Printf("auth: all sessions revoked for user %d (%s)", claims.UserID, claims.Username)
	} else if claims.ExpiresAt != nil {
		middleware.RevokeToken(claims.ID, claims.ExpiresAt.Time)
	} else {
		middleware.RevokeToken(claims.ID, time.Now().Add(config.GetConfig().AccessTokenTTL))
	}

	middleware.ClearAuthCookie(w)
	log.Printf("auth: logout user %d (%s) ip=%s", claims.UserID, claims.Username, middleware.ClientIP(r))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "message": "Logged out",
	})
}

// HandleSession reports who the caller is and what they may see. The frontend
// uses it to confirm a session is still live before rendering a machine page,
// instead of trusting a value it kept in local storage.
func HandleSession(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false, "message": "Authentication required",
		})
		return
	}

	tables, err := authorizedTables(r.Context(), claims)
	if err != nil {
		log.Printf("auth: could not load grants for user %d: %v", claims.UserID, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"success": false, "message": "Authorization data is temporarily unavailable",
		})
		return
	}

	machines := make([]map[string]string, 0, len(tables))
	for _, table := range tables {
		machines = append(machines, map[string]string{
			"machineName": machineStatusTables[table],
			"table":       table,
		})
	}

	var mustChange bool
	var stored sql.NullString
	if err := database.SafeQueryRowContext(r.Context(),
		"SELECT password FROM kabu_users WHERE id = ?", claims.UserID).Scan(&stored); err == nil {
		mustChange = strings.HasPrefix(stored.String, initialPasswordPrefix) || !isBcryptHash(stored.String)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user": map[string]interface{}{
			"id":          claims.UserID,
			"username":    claims.Username,
			"accountType": claims.AccountType,
		},
		"machines":           machines,
		"mustChangePassword": mustChange,
	})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// HandleChangePassword lets an authenticated user replace their own password.
//
// This is the endpoint the non-functional Profile menu item needed (S-04), and
// it is what makes mandatory rotation of admin-issued credentials possible
// (S-03). It re-checks the current password, so a stolen session cannot be used
// to take over the account outright.
func HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"success": false, "message": "Method not allowed",
		})
		return
	}

	claims, ok := middleware.GetUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false, "message": "Authentication required",
		})
		return
	}

	if ok, retryAfter := middleware.AllowRequest(
		"pwchange", middleware.ClientIP(r), passwordChangeAttempts, passwordChangeWindow); !ok {
		middleware.WriteRateLimited(w, retryAfter)
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "Invalid request body",
		})
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "Current and new password are required",
		})
		return
	}

	var stored sql.NullString
	if err := database.SafeQueryRowContext(r.Context(),
		"SELECT password FROM kabu_users WHERE id = ?", claims.UserID).Scan(&stored); err != nil {
		log.Printf("auth: change-password lookup failed for user %d: %v", claims.UserID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Server error",
		})
		return
	}

	if !VerifyPassword(stored.String, req.CurrentPassword).Valid {
		log.Printf("auth: change-password rejected (wrong current password) for user %d ip=%s",
			claims.UserID, middleware.ClientIP(r))
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false, "message": "Current password is incorrect",
		})
		return
	}

	if err := ValidatePasswordPolicy(req.NewPassword, claims.Username); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": err.Error(),
		})
		return
	}
	if VerifyPassword(stored.String, req.NewPassword).Valid {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "New password must differ from the current password",
		})
		return
	}

	// Stored without the INIT: marker: this one belongs to the user, so no
	// further rotation is demanded.
	hashed, err := HashPassword(req.NewPassword)
	if err != nil {
		log.Printf("auth: hashing failed for user %d: %v", claims.UserID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Server error",
		})
		return
	}

	if _, err := database.SafeExecContext(r.Context(),
		"UPDATE kabu_users SET password = ? WHERE id = ?", hashed, claims.UserID); err != nil {
		log.Printf("auth: password update failed for user %d: %v", claims.UserID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Could not update password",
		})
		return
	}

	// Every other session for this account dies with the old password, and the
	// caller has to sign in again with the new one.
	middleware.RevokeAllSessionsForUser(claims.UserID)
	middleware.ClearAuthCookie(w)
	log.Printf("auth: password changed for user %d (%s); all sessions revoked", claims.UserID, claims.Username)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"message":     "Password updated. Please sign in again.",
		"reloginRequired": true,
	})
}

// HandleRegister creates an account. It is mounted behind the authenticated
// admin router unless ALLOW_PUBLIC_REGISTER is set, because open registration on
// a plant-control API is an access-control hole.
func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"success": false, "message": "Method not allowed",
		})
		return
	}

	var regReq models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&regReq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "Invalid request body",
		})
		return
	}

	regReq.Username = strings.TrimSpace(regReq.Username)
	if regReq.Username == "" || regReq.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "Username and password are required",
		})
		return
	}
	if err := ValidatePasswordPolicy(regReq.Password, regReq.Username); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": err.Error(),
		})
		return
	}

	var existingID int
	err := database.SafeQueryRowContext(r.Context(),
		"SELECT id FROM kabu_users WHERE username = ? LIMIT 1", regReq.Username).Scan(&existingID)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"success": false, "message": "Username already exists. Please choose another.",
		})
		return
	}
	if err != sql.ErrNoRows {
		log.Printf("auth: username check error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Server error occurred during registration",
		})
		return
	}

	monitorAccessStr := strings.Join(regReq.MonitorAccess, ",")

	// Marked INIT: so the new owner must set their own password before the
	// credential the admin handed over stops working.
	hashed, err := HashInitialPassword(regReq.Password)
	if err != nil {
		log.Printf("auth: hashing failed during registration: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Server error occurred during registration",
		})
		return
	}

	insertQuery := `INSERT INTO kabu_users
		(accountType, firstName, lastName, username, email, phoneNumber, company, password, monitorAccess, location)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := database.SafeExecContext(r.Context(), insertQuery,
		regReq.AccountType, regReq.FirstName, regReq.LastName, regReq.Username,
		regReq.Email, regReq.PhoneNumber, regReq.Company, hashed,
		monitorAccessStr, regReq.Location,
	); err != nil {
		// The driver error is logged, not returned: it can carry schema details.
		log.Printf("auth: registration error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "Server error occurred during registration",
		})
		return
	}

	log.Printf("auth: account created username=%q by ip=%s", regReq.Username, middleware.ClientIP(r))
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"message": "User registered successfully. The user must set a new password at first sign-in.",
	})
}

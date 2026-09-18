package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"grain_backend/database"
	"grain_backend/middleware"
	"grain_backend/models"

	"github.com/gorilla/mux"
)

// userColumns is the SELECT list for user reads. Password is deliberately
// excluded so a hash (or, today, a plaintext password) can never leak through
// the admin UI.
const userColumns = "id, accountType, firstName, lastName, username, email, phoneNumber, company, monitorAccess, location, created_at"

// updatableUserFields maps a JSON request field to its database column. Only
// keys listed here can reach the UPDATE statement, so a client cannot rename
// its way into columns like id or created_at.
var updatableUserFields = map[string]string{
	"accountType":   "accountType",
	"firstName":     "firstName",
	"lastName":      "lastName",
	"username":      "username",
	"email":         "email",
	"phoneNumber":   "phoneNumber",
	"company":       "company",
	"password":      "password",
	"monitorAccess": "monitorAccess",
	"location":      "location",
}

// updateFieldOrder fixes the order fields are applied in so the generated SQL
// is deterministic (map iteration is not) and easy to read in logs.
var updateFieldOrder = []string{
	"accountType", "firstName", "lastName", "username", "email",
	"phoneNumber", "company", "password", "monitorAccess", "location",
}

// defaultUserAdmins are the only accounts allowed to use the user-management
// endpoints. The gate is by username, not accountType: both of these are
// accountType "manufactura", which other ordinary users also carry, so a role
// check would hand user management to everyone in that group.
const defaultUserAdmins = "Narayan12,Yogendra"

// userAdmins returns the allowlist. Override with USER_ADMIN_USERNAMES (comma
// separated) to grant or revoke access without a rebuild — matching is
// case-insensitive.
func userAdmins() map[string]bool {
	raw := os.Getenv("USER_ADMIN_USERNAMES")
	if strings.TrimSpace(raw) == "" {
		raw = defaultUserAdmins
	}
	allowed := make(map[string]bool)
	for _, u := range strings.Split(raw, ",") {
		if u = strings.ToLower(strings.TrimSpace(u)); u != "" {
			allowed[u] = true
		}
	}
	return allowed
}

func isUserAdmin(claims *middleware.UserClaims) bool {
	return userAdmins()[strings.ToLower(strings.TrimSpace(claims.Username))]
}

func writeUserJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeUserError(w http.ResponseWriter, status int, message string) {
	writeUserJSON(w, status, map[string]interface{}{
		"success": false,
		"message": message,
	})
}

// requireUserAdmin resolves the caller and enforces the username allowlist. It
// returns the caller's claims so handlers can also apply per-record rules.
func requireUserAdmin(w http.ResponseWriter, r *http.Request) (*middleware.UserClaims, bool) {
	claims, ok := middleware.GetUserFromContext(r)
	if !ok {
		writeUserError(w, http.StatusUnauthorized, "Authentication required")
		return nil, false
	}
	if !isUserAdmin(claims) {
		writeUserError(w, http.StatusForbidden, "You are not permitted to manage users")
		return nil, false
	}
	return claims, true
}

// userIDFromPath reads and validates the {id} path variable.
func userIDFromPath(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil || id <= 0 {
		writeUserError(w, http.StatusBadRequest, "Invalid user id")
		return 0, false
	}
	return id, true
}

// scanUser reads one row shaped like userColumns. Nullable columns come back as
// empty strings rather than failing the scan.
func scanUser(rows interface{ Scan(...interface{}) error }) (models.User, error) {
	var user models.User
	var createdAt sql.NullTime
	var accountType, firstName, lastName, email, phoneNumber, company, monitorAccess, location sql.NullString

	err := rows.Scan(
		&user.ID, &accountType, &firstName, &lastName,
		&user.Username, &email, &phoneNumber, &company,
		&monitorAccess, &location, &createdAt,
	)
	if err != nil {
		return user, err
	}

	user.AccountType = accountType.String
	user.FirstName = firstName.String
	user.LastName = lastName.String
	user.Email = email.String
	user.PhoneNumber = phoneNumber.String
	user.Company = company.String
	user.MonitorAccess = monitorAccess.String
	user.Location = location.String
	user.CreatedAt = createdAt.Time

	return user, nil
}

// HandleListUsers returns every user, newest first. Restricted to the
// allowlisted accounts.
// GET /api/users?search=&limit=&offset=
func HandleListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserAdmin(w, r); !ok {
		return
	}

	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	query := "SELECT " + userColumns + " FROM kabu_users"
	args := []interface{}{}

	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		query += " WHERE username LIKE ? OR firstName LIKE ? OR lastName LIKE ? OR email LIKE ? OR company LIKE ?"
		like := "%" + search + "%"
		args = append(args, like, like, like, like, like)
	}

	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.SafeQueryContext(r.Context(), query, args...)
	if err != nil {
		log.Printf("List users query error: %v", err)
		writeUserError(w, http.StatusInternalServerError, "Server error while fetching users")
		return
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			log.Printf("Error scanning user row: %v", err)
			writeUserError(w, http.StatusInternalServerError, "Server error while reading users")
			return
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating users: %v", err)
		writeUserError(w, http.StatusInternalServerError, "Server error while reading users")
		return
	}

	writeUserJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "Users retrieved successfully",
		"count":     len(users),
		"data":      users,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// updateUserRequest carries only the fields the client actually sent. Pointers
// (and json.RawMessage for monitorAccess) let a partial update leave untouched
// columns alone instead of blanking them.
type updateUserRequest struct {
	AccountType   *string         `json:"accountType"`
	FirstName     *string         `json:"firstName"`
	LastName      *string         `json:"lastName"`
	Username      *string         `json:"username"`
	Email         *string         `json:"email"`
	PhoneNumber   *string         `json:"phoneNumber"`
	Company       *string         `json:"company"`
	Password      *string         `json:"password"`
	MonitorAccess json.RawMessage `json:"monitorAccess"`
	Location      *string         `json:"location"`
}

// normalizeMonitorAccess accepts monitorAccess as either ["A","B"] or "A,B" —
// the register endpoint sends an array, the column stores a CSV string — and
// returns the CSV form.
func normalizeMonitorAccess(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return strings.Join(list, ","), nil
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str, nil
	}

	return "", errors.New("monitorAccess must be an array of strings or a comma-separated string")
}

// HandleUpdateUser edits one user. Restricted to the allowlisted accounts.
// PUT|PATCH /api/users/{id}
func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireUserAdmin(w, r)
	if !ok {
		return
	}

	id, validID := userIDFromPath(w, r)
	if !validID {
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeUserError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	values := map[string]interface{}{}
	if req.AccountType != nil {
		values["accountType"] = *req.AccountType
	}
	if req.FirstName != nil {
		values["firstName"] = *req.FirstName
	}
	if req.LastName != nil {
		values["lastName"] = *req.LastName
	}
	if req.Email != nil {
		values["email"] = *req.Email
	}
	if req.PhoneNumber != nil {
		values["phoneNumber"] = *req.PhoneNumber
	}
	if req.Company != nil {
		values["company"] = *req.Company
	}
	if req.Location != nil {
		values["location"] = *req.Location
	}
	if req.MonitorAccess != nil {
		access, err := normalizeMonitorAccess(req.MonitorAccess)
		if err != nil {
			writeUserError(w, http.StatusBadRequest, err.Error())
			return
		}
		values["monitorAccess"] = access
	}
	if req.Password != nil {
		if strings.TrimSpace(*req.Password) == "" {
			writeUserError(w, http.StatusBadRequest, "Password cannot be empty")
			return
		}
		if err := ValidatePasswordPolicy(*req.Password, ""); err != nil {
			writeUserError(w, http.StatusBadRequest, err.Error())
			return
		}
		// An admin reset is stored hashed and marked as admin-issued, so the
		// account owner has to replace it at next sign-in and the reset value
		// never sits in the database as cleartext (S-03, S-04).
		hashed, err := HashInitialPassword(*req.Password)
		if err != nil {
			log.Printf("Password hashing error for user %d: %v", id, err)
			writeUserError(w, http.StatusInternalServerError, "Server error while updating user")
			return
		}
		values["password"] = hashed
	}
	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username == "" {
			writeUserError(w, http.StatusBadRequest, "Username cannot be empty")
			return
		}
		// Reject a rename that would collide with a different account before
		// the UPDATE, so the client gets 409 instead of a raw driver error.
		dupRows, err := database.SafeQueryContext(r.Context(),
			"SELECT id FROM kabu_users WHERE username = ? AND id <> ? LIMIT 1", username, id)
		if err != nil {
			log.Printf("Username check error: %v", err)
			writeUserError(w, http.StatusInternalServerError, "Server error while updating user")
			return
		}
		taken := dupRows.Next()
		dupRows.Close()
		if taken {
			writeUserError(w, http.StatusConflict, "Username already exists. Please choose another.")
			return
		}
		values["username"] = username
	}

	if len(values) == 0 {
		writeUserError(w, http.StatusBadRequest, "No updatable fields supplied")
		return
	}

	setClauses := make([]string, 0, len(values))
	args := make([]interface{}, 0, len(values)+1)
	for _, field := range updateFieldOrder {
		val, sent := values[field]
		if !sent {
			continue
		}
		setClauses = append(setClauses, "`"+updatableUserFields[field]+"` = ?")
		args = append(args, val)
	}
	args = append(args, id)

	result, err := database.SafeExecContext(r.Context(),
		"UPDATE kabu_users SET "+strings.Join(setClauses, ", ")+" WHERE id = ?", args...)
	if err != nil {
		log.Printf("Update user %d error: %v", id, err)
		writeUserError(w, http.StatusInternalServerError, "Server error while updating user")
		return
	}

	// An access change must take effect now, not after the grant cache expires.
	if _, changed := values["monitorAccess"]; changed {
		InvalidateMachineGrant(id)
		log.Printf("authz: machine grants changed for user %d by %s", id, claims.Username)
	}
	// A reset password must not leave the old sessions usable.
	if _, changed := values["password"]; changed {
		middleware.RevokeAllSessionsForUser(id)
		log.Printf("auth: password reset for user %d by %s; all sessions revoked", id, claims.Username)
	}

	// RowsAffected is 0 both for "no such user" and for "values were already
	// identical", so confirm existence with a read instead of guessing.
	if _, err := result.RowsAffected(); err != nil {
		log.Printf("Update user %d rows-affected error: %v", id, err)
	}

	rows, err := database.SafeQueryContext(r.Context(),
		"SELECT "+userColumns+" FROM kabu_users WHERE id = ?", id)
	if err != nil {
		log.Printf("Reload user %d error: %v", id, err)
		writeUserError(w, http.StatusInternalServerError, "User updated but could not be reloaded")
		return
	}
	defer rows.Close()

	if !rows.Next() {
		writeUserError(w, http.StatusNotFound, "User not found")
		return
	}
	user, err := scanUser(rows)
	if err != nil {
		log.Printf("Error scanning updated user %d: %v", id, err)
		writeUserError(w, http.StatusInternalServerError, "User updated but could not be reloaded")
		return
	}

	writeUserJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "User updated successfully",
		"data":    user,
	})
}

// HandleDeleteUser removes one user. Restricted to the allowlisted accounts,
// and such an account cannot delete itself — that would leave the caller
// holding a token for a user that no longer exists, and would strip user
// management from the allowlist entry.
// DELETE /api/users/{id}
func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireUserAdmin(w, r)
	if !ok {
		return
	}

	id, ok := userIDFromPath(w, r)
	if !ok {
		return
	}

	if claims.UserID == id {
		writeUserError(w, http.StatusForbidden, "You cannot delete your own account")
		return
	}

	result, err := database.SafeExecContext(r.Context(), "DELETE FROM kabu_users WHERE id = ?", id)
	if err != nil {
		log.Printf("Delete user %d error: %v", id, err)
		writeUserError(w, http.StatusInternalServerError, "Server error while deleting user")
		return
	}

	affected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Delete user %d rows-affected error: %v", id, err)
		affected = 1 // the DELETE itself succeeded; don't report a false 404
	}
	if affected == 0 {
		writeUserError(w, http.StatusNotFound, "User not found")
		return
	}

	writeUserJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "User deleted successfully",
		"id":      id,
	})
}

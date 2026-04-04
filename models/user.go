package models

import "time"

// User represents a user in the system
type User struct {
	ID            int       `json:"id"`
	AccountType   string    `json:"accountType"`
	FirstName     string    `json:"firstName"`
	LastName      string    `json:"lastName"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	PhoneNumber   string    `json:"phoneNumber"`
	Company       string    `json:"company"`
	Password      string    `json:"-"` // Never return in JSON
	MonitorAccess string    `json:"monitorAccess"`
	Location      string    `json:"location"`
	CreatedAt     time.Time `json:"created_at"`
}

// LoginRequest represents a login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterRequest represents a registration request body
type RegisterRequest struct {
	AccountType   string   `json:"accountType"`
	FirstName     string   `json:"firstName"`
	LastName      string   `json:"lastName"`
	Username      string   `json:"username"`
	Email         string   `json:"email"`
	PhoneNumber   string   `json:"phoneNumber"`
	Company       string   `json:"company"`
	Password      string   `json:"password"`
	MonitorAccess []string `json:"monitorAccess"`
	Location      string   `json:"location"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Message string `json:"message"`
	User    *User  `json:"user"`
	Token   string `json:"token,omitempty"`
}

// APIResponse is a generic API response
type APIResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// Temporary test: proves mustChangePassword is true until the user changes their
// password, then false on every subsequent login. Creates a throwaway account,
// exercises the flow, and deletes it. Non-destructive to real accounts.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"grain_backend/config"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

const base = "http://localhost:3000"

var db *sql.DB

func main() {
	_ = godotenv.Load("../.env")
	cfg := config.GetConfig()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	const uname = "zz_pwflow_test"
	const p1 = "InitialPass123"
	const p2 = "MyChosenPass456"
	const p3 = "ThirdPassword789"

	// Clean any leftover, then create the account with a plaintext password —
	// exactly the state the real accounts are in today.
	db.Exec("DELETE FROM kabu_users WHERE username = ?", uname)
	_, err = db.Exec(`INSERT INTO kabu_users
	    (accountType, firstName, lastName, username, email, phoneNumber, company, password, monitorAccess, location)
	    VALUES ('customer','PW','Flow',?, 'pwflow@test.local','0','TestCo', ?, 'GTPL_121','TestLoc')`, uname, p1)
	if err != nil {
		panic(err)
	}
	defer db.Exec("DELETE FROM kabu_users WHERE username = ?", uname)

	fmt.Println("Account created with a PLAINTEXT password (like the live accounts).")
	fmt.Println(strings.Repeat("-", 60))

	// 1. First login — must prompt.
	tok, must := login(uname, p1)
	fmt.Printf("1) login with initial password       -> mustChangePassword=%v  (expect true)\n", must)
	fmt.Printf("   stored form now: %s\n", storedForm(uname))

	// 2. Second login BEFORE changing — should still prompt (nothing changed).
	tok, must = login(uname, p1)
	fmt.Printf("2) login again, still not changed     -> mustChangePassword=%v  (expect true)\n", must)

	// 3. Change the password.
	code, body := changePassword(tok, p1, p2)
	fmt.Printf("3) change password                    -> %d %s\n", code, truncate(body, 70))
	fmt.Printf("   stored form now: %s\n", storedForm(uname))

	// 4. Login with the NEW password — must NOT prompt.
	tok, must = login(uname, p2)
	fmt.Printf("4) login with new password            -> mustChangePassword=%v  (expect false)\n", must)

	// 5. Login yet again — still must NOT prompt.
	tok, must = login(uname, p2)
	fmt.Printf("5) login again with new password      -> mustChangePassword=%v  (expect false)\n", must)

	// 6. Change again, then login — still must NOT prompt.
	changePassword(tok, p2, p3)
	tok, must = login(uname, p3)
	fmt.Printf("6) after a SECOND change, login       -> mustChangePassword=%v  (expect false)\n", must)

	// 7. Old password must no longer work.
	_, body7 := loginRaw(uname, p1)
	fmt.Printf("7) login with the OLD password        -> %s  (expect Invalid)\n", truncate(body7, 55))

	fmt.Println(strings.Repeat("-", 60))
	_ = tok
	fmt.Println("[throwaway account deleted]")
}

func login(u, p string) (string, bool) {
	body := loginBody(u, p)
	var m map[string]interface{}
	json.Unmarshal([]byte(body), &m)
	tok, _ := m["token"].(string)
	must, _ := m["mustChangePassword"].(bool)
	return tok, must
}

func loginRaw(u, p string) (string, string) { return "", loginBody(u, p) }

func loginBody(u, p string) string {
	payload, _ := json.Marshal(map[string]string{"username": u, "password": p})
	resp, err := http.Post(base+"/api/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "ERR " + err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func changePassword(token, current, next string) (int, string) {
	payload, _ := json.Marshal(map[string]string{"currentPassword": current, "newPassword": next})
	req, _ := http.NewRequest("POST", base+"/api/auth/change-password", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func storedForm(uname string) string {
	var pw sql.NullString
	db.QueryRow("SELECT password FROM kabu_users WHERE username = ?", uname).Scan(&pw)
	switch {
	case strings.HasPrefix(pw.String, "INIT:"):
		return "INIT: bcrypt (admin-issued, must rotate)"
	case strings.HasPrefix(pw.String, "$2"):
		return "bcrypt (user-chosen, no rotation)"
	default:
		return "PLAINTEXT"
	}
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

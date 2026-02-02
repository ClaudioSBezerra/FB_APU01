package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// --- Structs ---

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	FullName    string    `json:"full_name"`
	IsVerified  bool      `json:"is_verified"`
	TrialEndsAt time.Time `json:"trial_ends_at"`
	CreatedAt   string    `json:"created_at"`
}

type RegisterRequest struct {
	FullName    string `json:"full_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	CompanyName string `json:"company_name"`
	CNPJ        string `json:"cnpj"` // Optional
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
	// Context info for the footer
	Environment string `json:"environment_name"`
	Group       string `json:"group_name"`
	Company     string `json:"company_name"`
}

// --- Utils ---

var jwtSecret = []byte(getEnv("JWT_SECRET", "super-secret-key-change-me-in-prod"))

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24).Unix(), // 24 hours
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// --- Handlers ---

func RegisterHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Email == "" || req.Password == "" || req.FullName == "" || req.CompanyName == "" {
			http.Error(w, "Missing required fields", http.StatusBadRequest)
			return
		}

		// 1. Hash Password
		hash, err := HashPassword(req.Password)
		if err != nil {
			http.Error(w, "Error hashing password", http.StatusInternalServerError)
			return
		}

		// 2. Start Transaction
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 3. Create User
		var userID string
		trialEnds := time.Now().Add(time.Hour * 24 * 14) // 14 days
		err = tx.QueryRow(`
			INSERT INTO users (email, password_hash, full_name, trial_ends_at, is_verified)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, req.Email, hash, req.FullName, trialEnds, false).Scan(&userID) // Not verified by default

		if err != nil {
			http.Error(w, "Error creating user (email might be taken)", http.StatusConflict)
			return
		}

		// 4. Get Default Environment and Group
		var envID, groupID, envName, groupName string

		// Fetch default environment
		err = tx.QueryRow("SELECT id, name FROM environments WHERE name = 'Ambiente de Testes' LIMIT 1").Scan(&envID, &envName)
		if err != nil {
			// Try to find ANY environment if default is missing (fallback)
			err = tx.QueryRow("SELECT id, name FROM environments LIMIT 1").Scan(&envID, &envName)
			if err != nil {
				http.Error(w, "No environments available. System setup required.", http.StatusInternalServerError)
				return
			}
		}

		// Fetch default group in that environment
		err = tx.QueryRow("SELECT id, name FROM enterprise_groups WHERE environment_id = $1 AND name = 'Grupo de Empresas Testes' LIMIT 1", envID).Scan(&groupID, &groupName)
		if err != nil {
			// Try to find ANY group if default is missing
			err = tx.QueryRow("SELECT id, name FROM enterprise_groups WHERE environment_id = $1 LIMIT 1", envID).Scan(&groupID, &groupName)
			if err != nil {
				http.Error(w, "No groups available. System setup required.", http.StatusInternalServerError)
				return
			}
		}

		// 5. Link User to Environment
		_, err = tx.Exec("INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'admin')", userID, envID)
		if err != nil {
			http.Error(w, "Error linking user to environment", http.StatusInternalServerError)
			return
		}

		// 6. Create Company with owner_id
		var companyID string
		err = tx.QueryRow(`
			INSERT INTO companies (group_id, cnpj, name, trade_name, owner_id)
			VALUES ($1, $2, $3, $3, $4)
			RETURNING id
		`, groupID, req.CNPJ, req.CompanyName, userID).Scan(&companyID)
		if err != nil {
			http.Error(w, "Error creating company", http.StatusInternalServerError)
			return
		}

		// Commit
		if err := tx.Commit(); err != nil {
			http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
			return
		}

		// Generate Token
		token, _ := GenerateToken(userID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{
			Token: token,
			User: User{
				ID:          userID,
				Email:       req.Email,
				FullName:    req.FullName,
				IsVerified:  false,
				TrialEndsAt: trialEnds,
			},
			Environment: envName,
			Group:       groupName,
			Company:     req.CompanyName,
		})
	}
}

func LoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Get User
		var user User
		var hash string
		err := db.QueryRow(`
			SELECT id, email, full_name, password_hash, is_verified, trial_ends_at, created_at
			FROM users WHERE email = $1
		`, req.Email).Scan(&user.ID, &user.Email, &user.FullName, &hash, &user.IsVerified, &user.TrialEndsAt, &user.CreatedAt)

		if err == sql.ErrNoRows {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// Check Password
		if !CheckPasswordHash(req.Password, hash) {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Generate Token
		token, err := GenerateToken(user.ID)
		if err != nil {
			http.Error(w, "Error generating token", http.StatusInternalServerError)
			return
		}

		// Get Context Info (Try to find company owned by user)
		var envName, groupName, companyName string

		err = db.QueryRow(`
			SELECT e.name, eg.name, c.name
			FROM companies c
			JOIN enterprise_groups eg ON c.group_id = eg.id
			JOIN environments e ON eg.environment_id = e.id
			WHERE c.owner_id = $1
			ORDER BY c.created_at DESC
			LIMIT 1
		`, user.ID).Scan(&envName, &groupName, &companyName)

		if err == sql.ErrNoRows {
			// No company owned, try to find one they have access to via environment?
			// For now, leave empty or set placeholders
			envName = "Sem Ambiente"
			groupName = "Sem Grupo"
			companyName = "Sem Empresa"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{
			Token:       token,
			User:        user,
			Environment: envName,
			Group:       groupName,
			Company:     companyName,
		})
	}
}

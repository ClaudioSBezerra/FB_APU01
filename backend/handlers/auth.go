package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// --- Structs ---

type contextKey string

const ClaimsKey contextKey = "claims"

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	FullName    string    `json:"full_name"`
	IsVerified  bool      `json:"is_verified"`
	TrialEndsAt time.Time `json:"trial_ends_at"`
	Role        string    `json:"role"`
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
	CNPJ        string `json:"cnpj"` // Added CNPJ for company context
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

func GenerateToken(userID, role string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(time.Hour * 24).Unix(), // 24 hours
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// --- Handlers ---

func AuthMiddleware(next http.HandlerFunc, requiredRole string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		tokenString := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		} else {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
			return
		}

		// Check role
		userRole, ok := claims["role"].(string)
		if !ok {
			http.Error(w, "Role not found in token", http.StatusUnauthorized)
			return
		}

		if requiredRole != "" && userRole != requiredRole && userRole != "admin" {
			http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func GetUserIDFromContext(r *http.Request) string {
	claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
	if !ok {
		return ""
	}
	userID, ok := claims["user_id"].(string)
	if !ok {
		return ""
	}
	return userID
}

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
		var role string
		trialEnds := time.Now().Add(time.Hour * 24 * 14) // 14 days
		// Ensure role column is populated (default 'user') and returned
		err = tx.QueryRow(`
			INSERT INTO users (email, password_hash, full_name, trial_ends_at, is_verified, role)
			VALUES ($1, $2, $3, $4, $5, 'user')
			RETURNING id, role
		`, req.Email, hash, req.FullName, trialEnds, false).Scan(&userID, &role)

		if err != nil {
			// Check specifically for unique constraint violation
			tx.Rollback() // Ensure rollback before returning
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode("User already registered")
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
		token, _ := GenerateToken(userID, "user")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{
			Token: token,
			User: User{
				ID:          userID,
				Email:       req.Email,
				FullName:    req.FullName,
				IsVerified:  false,
				TrialEndsAt: trialEnds,
				Role:        "user",
			},
			Environment: envName,
			Group:       groupName,
			Company:     req.CompanyName,
			CNPJ:        req.CNPJ,
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
		// Use COALESCE for role to handle cases where migration might have issues (though it should have default)
		err := db.QueryRow(`
			SELECT id, email, full_name, password_hash, is_verified, trial_ends_at, COALESCE(role, 'user'), created_at
			FROM users WHERE email = $1
		`, req.Email).Scan(&user.ID, &user.Email, &user.FullName, &hash, &user.IsVerified, &user.TrialEndsAt, &user.Role, &user.CreatedAt)

		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode("Invalid credentials")
			return
		} else if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode("Database error")
			return
		}

		// Check Password
		if !CheckPasswordHash(req.Password, hash) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode("Invalid credentials")
			return
		}

		// 2. Generate Token
		token, err := GenerateToken(user.ID, user.Role)
		if err != nil {
			http.Error(w, "Error generating token", http.StatusInternalServerError)
			return
		}

		// Get Context Info (Try to find company owned by user)
		var envName, groupName, companyName, companyCNPJ string

		err = db.QueryRow(`
			SELECT e.name, eg.name, c.name, c.cnpj
			FROM companies c
			JOIN enterprise_groups eg ON c.group_id = eg.id
			JOIN environments e ON eg.environment_id = e.id
			WHERE c.owner_id = $1
			ORDER BY c.created_at DESC
			LIMIT 1
		`, user.ID).Scan(&envName, &groupName, &companyName, &companyCNPJ)

		if err == sql.ErrNoRows {
			// No company owned, try to find one they have access to via environment?
			// For now, leave empty or set placeholders
			envName = "Sem Ambiente"
			groupName = "Sem Grupo"
			companyName = "Sem Empresa"
			companyCNPJ = ""
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{
			Token:       token,
			User:        user,
			Environment: envName,
			Group:       groupName,
			Company:     companyName,
			CNPJ:        companyCNPJ,
		})
	}
}

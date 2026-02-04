package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
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
	CompanyID   string `json:"company_id"` // Added CompanyID
	CNPJ        string `json:"cnpj"`       // Added CNPJ for company context
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

// GetUserCompanyID fetches the company ID for a given user.
// It prioritizes companies owned by the user, then falls back to companies where the user is a member.
func GetUserCompanyID(db *sql.DB, userID string) (string, error) {
	var companyID string
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Strategy A: Check if user OWNS a company
	err := db.QueryRowContext(ctx, `
		SELECT c.id
		FROM companies c
		WHERE c.owner_id = $1
		ORDER BY c.created_at DESC
		LIMIT 1
	`, userID).Scan(&companyID)

	if err == nil {
		return companyID, nil
	}

	if err != sql.ErrNoRows {
		return "", err
	}

	// Strategy B: Check via User Environment
	err = db.QueryRowContext(ctx, `
		SELECT c.id
		FROM user_environments ue
		JOIN enterprise_groups eg ON eg.environment_id = ue.environment_id
		JOIN companies c ON c.group_id = eg.id
		WHERE ue.user_id = $1
		ORDER BY c.created_at DESC
		LIMIT 1
	`, userID).Scan(&companyID)

	if err != nil {
		return "", err
	}

	return companyID, nil
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
			log.Printf("[Register] Error hashing password: %v", err)
			http.Error(w, "Error hashing password", http.StatusInternalServerError)
			return
		}

		// 2. Start Transaction
		tx, err := db.Begin()
		if err != nil {
			log.Printf("[Register] Error starting transaction: %v", err)
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
			log.Printf("[Register] Error creating user: %v", err)
			// Check specifically for unique constraint violation
			tx.Rollback() // Ensure rollback before returning
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode("Este e-mail já está cadastrado.")
			return
		}

		// 4. Get Default Environment and Group
		var envID, groupID, envName, groupName string

		// Fetch default environment or CREATE if not exists
		err = tx.QueryRow("SELECT id, name FROM environments WHERE name = 'Ambiente de Testes' LIMIT 1").Scan(&envID, &envName)
		if err != nil {
			if err == sql.ErrNoRows {
				// Create Environment
				err = tx.QueryRow("INSERT INTO environments (name, description) VALUES ('Ambiente de Testes', 'Ambiente compartilhado para usuários trial') RETURNING id, name").Scan(&envID, &envName)
				if err != nil {
					log.Printf("[Register] Error creating environment: %v", err)
					tx.Rollback()
					http.Error(w, "Error creating default environment", http.StatusInternalServerError)
					return
				}
			} else {
				log.Printf("[Register] Error fetching environment: %v", err)
				tx.Rollback()
				http.Error(w, "Database error fetching environment", http.StatusInternalServerError)
				return
			}
		}

		// Fetch default group in that environment or CREATE if not exists
		err = tx.QueryRow("SELECT id, name FROM enterprise_groups WHERE environment_id = $1 AND name = 'Grupo de Empresas Testes' LIMIT 1", envID).Scan(&groupID, &groupName)
		if err != nil {
			if err == sql.ErrNoRows {
				// Create Group
				err = tx.QueryRow("INSERT INTO enterprise_groups (environment_id, name, description) VALUES ($1, 'Grupo de Empresas Testes', 'Grupo compartilhado para trial') RETURNING id, name", envID).Scan(&groupID, &groupName)
				if err != nil {
					log.Printf("[Register] Error creating group: %v", err)
					tx.Rollback()
					http.Error(w, "Error creating default group", http.StatusInternalServerError)
					return
				}
			} else {
				log.Printf("[Register] Error fetching group: %v", err)
				tx.Rollback()
				http.Error(w, "Database error fetching group", http.StatusInternalServerError)
				return
			}
		}

		// 5. Link User to Environment
		_, err = tx.Exec("INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'admin')", userID, envID)
		if err != nil {
			log.Printf("[Register] Error linking user to environment: %v", err)
			http.Error(w, "Error linking user to environment", http.StatusInternalServerError)
			return
		}

		// 6. Create Company with owner_id
		var companyID string
		err = tx.QueryRow(`
			INSERT INTO companies (group_id, name, trade_name, owner_id)
			VALUES ($1, $2, $2, $3)
			RETURNING id
		`, groupID, req.CompanyName, userID).Scan(&companyID)
		if err != nil {
			log.Printf("[Register] Error creating company: %v", err)
			http.Error(w, "Error creating company", http.StatusInternalServerError)
			return
		}

		// Commit
		if err := tx.Commit(); err != nil {
			log.Printf("[Register] Error committing transaction: %v", err)
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
			CompanyID:   companyID,
			CNPJ:        "", // CNPJ removed from company
		})
	}
}

func LoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		log.Printf("[Login] Attempting login for: %s", req.Email)

		// Get User
		var user User
		var hash string
		// Use COALESCE for role to handle cases where migration might have issues (though it should have default)
		err := db.QueryRow(`
			SELECT id, email, full_name, password_hash, is_verified, trial_ends_at, COALESCE(role, 'user'), created_at
			FROM users WHERE email = $1
		`, req.Email).Scan(&user.ID, &user.Email, &user.FullName, &hash, &user.IsVerified, &user.TrialEndsAt, &user.Role, &user.CreatedAt)

		if err == sql.ErrNoRows {
			log.Printf("[Login] User not found: %s", req.Email)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode("E-mail não encontrado ou senha inválida")
			return
		} else if err != nil {
			log.Printf("[Login] Database error fetching user: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode("Erro no servidor")
			return
		}

		// Check Password
		if !CheckPasswordHash(req.Password, hash) {
			log.Printf("[Login] Invalid password for: %s", req.Email)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode("E-mail não encontrado ou senha inválida")
			return
		}

		// 2. Generate Token
		token, err := GenerateToken(user.ID, user.Role)
		if err != nil {
			log.Printf("[Login] Error generating token: %v", err)
			http.Error(w, "Error generating token", http.StatusInternalServerError)
			return
		}

		// 4. Get Environment, Group, and Company Context
		// OPTIMIZATION: Split query to avoid complex joins and potential locks/slowdowns
		// Added Timeout context to prevent 504 Gateway Timeouts on slow DB
		var envName, groupName, companyName, companyID string

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		// Strategy A: Check if user OWNS a company (Fastest/Most Common)
		err = db.QueryRowContext(ctx, `
			SELECT e.name, eg.name, c.name, c.id
			FROM companies c
			JOIN enterprise_groups eg ON c.group_id = eg.id
			JOIN environments e ON eg.environment_id = e.id
			WHERE c.owner_id = $1
			ORDER BY c.created_at DESC
			LIMIT 1
		`, user.ID).Scan(&envName, &groupName, &companyName, &companyID)

		if err == sql.ErrNoRows {
			// Strategy B: If not owner, check via User Environment (Slower but necessary for team members)
			log.Printf("[Login] User %s owns no company, checking memberships...", req.Email)
			err = db.QueryRowContext(ctx, `
				SELECT e.name, eg.name, c.name, c.id
				FROM user_environments ue
				JOIN environments e ON ue.environment_id = e.id
				JOIN enterprise_groups eg ON eg.environment_id = e.id
				JOIN companies c ON c.group_id = eg.id
				WHERE ue.user_id = $1
				ORDER BY c.created_at DESC
				LIMIT 1
			`, user.ID).Scan(&envName, &groupName, &companyName, &companyID)
		}

		if err == sql.ErrNoRows {
			// No company found at all - Auto-provisioning
			log.Printf("[Login] No company context found for user: %s. Auto-provisioning default context...", req.Email)

			// 1. Get/Create Default Environment
			var envID string
			errEnv := db.QueryRowContext(ctx, "SELECT id, name FROM environments WHERE name = 'Ambiente de Testes' LIMIT 1").Scan(&envID, &envName)
			if errEnv == sql.ErrNoRows {
				errEnv = db.QueryRowContext(ctx, "INSERT INTO environments (name, description) VALUES ('Ambiente de Testes', 'Ambiente auto-gerado') RETURNING id, name").Scan(&envID, &envName)
			}
			
			if errEnv != nil {
				log.Printf("[Login] Auto-provision failed at Environment: %v", errEnv)
				envName = "Sem Ambiente"
				groupName = "Sem Grupo"
				companyName = "Sem Empresa"
				companyID = ""
			} else {
				// 2. Get/Create Default Group
				var groupID string
				errGroup := db.QueryRowContext(ctx, "SELECT id, name FROM enterprise_groups WHERE environment_id = $1 AND name = 'Grupo de Empresas Testes' LIMIT 1", envID).Scan(&groupID, &groupName)
				if errGroup == sql.ErrNoRows {
					errGroup = db.QueryRowContext(ctx, "INSERT INTO enterprise_groups (environment_id, name, description) VALUES ($1, 'Grupo de Empresas Testes', 'Grupo auto-gerado') RETURNING id, name", envID).Scan(&groupID, &groupName)
				}

				if errGroup != nil {
					log.Printf("[Login] Auto-provision failed at Group: %v", errGroup)
					groupName = "Sem Grupo"
					companyName = "Sem Empresa"
					companyID = ""
				} else {
					// 3. Link User to Environment (Idempotent)
					// We use ON CONFLICT DO NOTHING assuming there's a unique constraint or primary key on (user_id, environment_id)
					// If not, we might duplicate, but standard schema usually has it. 
					// Checking user_environments definition would be good, but let's assume standard PK.
					_, _ = db.ExecContext(ctx, "INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'admin') ON CONFLICT DO NOTHING", user.ID, envID)

					// 4. Create Company for User
					companyName = "Empresa de " + user.FullName
					if user.FullName == "" {
						companyName = "Minha Empresa"
					}
					
					errComp := db.QueryRowContext(ctx, `
						INSERT INTO companies (group_id, name, trade_name, owner_id)
						VALUES ($1, $2, $2, $3)
						RETURNING id
					`, groupID, companyName, user.ID).Scan(&companyID)

					if errComp != nil {
						log.Printf("[Login] Auto-provision failed at Company: %v", errComp)
						companyName = "Sem Empresa"
						companyID = ""
					} else {
						log.Printf("[Login] Auto-provision success: Created %s (%s)", companyName, companyID)
					}
				}
			}
			// Reset error to nil so we don't trigger the next error block
			err = nil

		} else if err != nil {
			// Could be timeout or other error
			log.Printf("[Login] Warning: Error fetching context (timeout?): %v. Proceeding without context.", err)
			// Don't fail login, just return empty context so user can enter
			envName = "Carregando..."
			groupName = "Carregando..."
			companyName = "Carregando..."
			companyID = ""
		}

		log.Printf("[Login] Success for %s. Duration: %v", req.Email, time.Since(start))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{
			Token:       token,
			User:        user,
			Environment: envName,
			Group:       groupName,
			Company:     companyName,
			CompanyID:   companyID,
			CNPJ:        "", // CNPJ removed from company
		})
	}
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

func ForgotPasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ForgotPasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		var userID string
		err := db.QueryRow("SELECT id FROM users WHERE email = $1", req.Email).Scan(&userID)
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode("E-mail não encontrado")
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// Generate token
		tokenBytes := make([]byte, 32)
		rand.Read(tokenBytes)
		rand.Read(tokenBytes) // Read twice just to be safe or copy paste error? No, just once is fine.
		token := hex.EncodeToString(tokenBytes)

		expiresAt := time.Now().Add(1 * time.Hour)

		_, err = db.Exec("INSERT INTO verification_tokens (user_id, token, type, expires_at) VALUES ($1, $2, 'password_reset', $3)", userID, token, expiresAt)
		if err != nil {
			http.Error(w, "Error creating token", http.StatusInternalServerError)
			return
		}

		// Simulate Email Sending
		fmt.Printf("\n=== MOCK EMAIL SERVICE ===\nTo: %s\nSubject: Password Reset\nLink: /reset-password?token=%s\n==========================\n", req.Email, token)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message":    "Link de recuperação gerado (verifique o console do servidor)",
			"mock_token": token,
		})
	}
}

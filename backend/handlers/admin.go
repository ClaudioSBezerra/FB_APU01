package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ResetCompanyDataRequest struct
type ResetCompanyDataRequest struct {
	CompanyID string `json:"company_id"`
}

// ResetCompanyDataHandler deletes all import jobs for a specific Company ID
// It allows users to clean their own company data, or admins to clean any company.
func ResetCompanyDataHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ResetCompanyDataRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.CompanyID == "" {
			http.Error(w, "Company ID is required", http.StatusBadRequest)
			return
		}

		// Get User Context
		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID := claims["user_id"].(string)
		role := claims["role"].(string)

		// Authorization Check: Must be Admin OR Environment Admin for the company
		if role != "admin" {
			var exists bool
			// Check if user has 'admin' role in the environment that owns the company
			err := db.QueryRow(`
				SELECT EXISTS(
					SELECT 1 
					FROM companies c
					JOIN enterprise_groups eg ON c.group_id = eg.id
					JOIN user_environments ue ON ue.environment_id = eg.environment_id
					WHERE ue.user_id = $1 
					  AND c.id = $2 
					  AND ue.role = 'admin'
				)`, userID, req.CompanyID).Scan(&exists)

			if err != nil {
				log.Printf("Error checking permission: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if !exists {
				http.Error(w, "Forbidden: You do not have permission to reset this company's data", http.StatusForbidden)
				return
			}
		}

		log.Printf("ResetCompanyData: User %s deleting data for CompanyID %s", userID, req.CompanyID)

		// Execute Deletion
		res, err := db.Exec("DELETE FROM import_jobs WHERE company_id = $1", req.CompanyID)
		if err != nil {
			log.Printf("Error deleting jobs for CompanyID %s: %v", req.CompanyID, err)
			http.Error(w, "Failed to delete company data", http.StatusInternalServerError)
			return
		}

		rowsDeleted, _ := res.RowsAffected()
		log.Printf("ResetCompanyData: Deleted %d jobs for CompanyID %s", rowsDeleted, req.CompanyID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":      "Company data deleted successfully",
			"jobs_deleted": rowsDeleted,
		})
	}
}

// ResetDatabaseHandler deletes all records from import_jobs,
// which cascades to all related SPED data tables (participants, regs, aggregations).
// It preserves system configuration tables like cfop and tabela_aliquotas.
func ResetDatabaseHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		log.Println("Admin: Initiating full database reset (clearing imported data)...")

		// Execute the deletion in a transaction for safety
		tx, err := db.Begin()
		if err != nil {
			log.Printf("Error starting transaction: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Optimize: Use TRUNCATE CASCADE for instant clearing of large datasets.
		// TRUNCATE is much faster than DELETE because it doesn't scan tables or log individual row deletions.
		// CASCADE ensures all dependent tables (reg_*, aggregations) are also cleared.
		_, err = tx.Exec("TRUNCATE TABLE import_jobs CASCADE")
		if err != nil {
			log.Printf("Error truncating import_jobs: %v", err)
			// Fallback to DELETE if TRUNCATE fails (e.g. permissions)
			_, err = tx.Exec("DELETE FROM import_jobs")
			if err != nil {
				log.Printf("Error deleting import_jobs (fallback): %v", err)
				http.Error(w, "Failed to reset database", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Error committing transaction: %v", err)
			http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
			return
		}

		log.Printf("Database reset successful (TRUNCATE).")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":      "Database reset successfully",
			"jobs_deleted": -1, // TRUNCATE doesn't return count
		})
	}
}

// ListUsersHandler returns all users (Admin only)
func ListUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, email, full_name, is_verified, trial_ends_at, role, created_at 
			FROM users ORDER BY created_at DESC
		`)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var u User
			if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.IsVerified, &u.TrialEndsAt, &u.Role, &u.CreatedAt); err != nil {
				continue
			}
			users = append(users, u)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}
}

// PromoteUserRequest struct
type PromoteUserRequest struct {
	Role       string `json:"role"`        // 'admin' or 'user'
	ExtendDays int    `json:"extend_days"` // Days to add to trial
}

// PromoteUserHandler updates user role or trial (Admin only)
func PromoteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("id")
		if userID == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		var req PromoteUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Update logic
		if req.Role != "" {
			_, err := db.Exec("UPDATE users SET role = $1 WHERE id = $2", req.Role, userID)
			if err != nil {
				http.Error(w, "Failed to update role", http.StatusInternalServerError)
				return
			}
		}

		if req.ExtendDays > 0 {
			// Get current trial end
			var currentEnd time.Time
			err := db.QueryRow("SELECT trial_ends_at FROM users WHERE id = $1", userID).Scan(&currentEnd)
			if err != nil {
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}

			// If expired, start from now. If not, add to existing.
			if currentEnd.Before(time.Now()) {
				currentEnd = time.Now()
			}
			newEnd := currentEnd.Add(time.Duration(req.ExtendDays) * 24 * time.Hour)

			_, err = db.Exec("UPDATE users SET trial_ends_at = $1 WHERE id = $2", newEnd, userID)
			if err != nil {
				http.Error(w, "Failed to update trial", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"})
	}
}

// DeleteUserHandler deletes a user and all their data (Admin only)
func DeleteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("id")
		if userID == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		_, err := db.Exec("DELETE FROM users WHERE id = $1", userID)
		if err != nil {
			http.Error(w, "Failed to delete user", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
	}
}

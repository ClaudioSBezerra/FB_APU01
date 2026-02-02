package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

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

		// Delete all jobs. 
		// Due to ON DELETE CASCADE constraints defined in migrations,
		// this will automatically delete:
		// - participants
		// - reg_0140, reg_c010, reg_c100, reg_c190, reg_c500, reg_c600, reg_d100, reg_d500, reg_d590, reg_9900
		// - operacoes_comerciais, energia_agregado, frete_agregado, comunicacoes_agregado
		result, err := tx.Exec("DELETE FROM import_jobs")
		if err != nil {
			log.Printf("Error deleting import_jobs: %v", err)
			http.Error(w, "Failed to reset database", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Error committing transaction: %v", err)
			http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
			return
		}

		rowsAffected, _ := result.RowsAffected()
		log.Printf("Database reset successful. Deleted %d jobs and their related data.", rowsAffected)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Database reset successfully",
			"jobs_deleted": rowsAffected,
		})
	}
}

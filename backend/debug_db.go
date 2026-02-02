package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	connStr := "postgres://postgres:postgres@localhost:5432/fiscal_db?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 1. Check User Iolanda
	fmt.Println("--- USERS (iolanda) ---")
	rows, err := db.Query("SELECT id, email, created_at FROM users WHERE email LIKE 'iolanda%'")
	if err != nil { log.Fatal(err) }
	defer rows.Close()
	var userID string
	for rows.Next() {
		var email, created string
		rows.Scan(&userID, &email, &created)
		fmt.Printf("ID: %s | Email: %s | Created: %s\n", userID, email, created)
	}

	// 2. Check Environments
	fmt.Println("\n--- ENVIRONMENTS ---")
	rowsEnv, _ := db.Query("SELECT id, name FROM environments")
	defer rowsEnv.Close()
	for rowsEnv.Next() {
		var id, name string
		rowsEnv.Scan(&id, &name)
		fmt.Printf("EnvID: %s | Name: %s\n", id, name)
	}

	// 3. Check Groups
	fmt.Println("\n--- GROUPS ---")
	rowsGrp, _ := db.Query("SELECT id, environment_id, name FROM enterprise_groups")
	defer rowsGrp.Close()
	for rowsGrp.Next() {
		var id, envID, name string
		rowsGrp.Scan(&id, &envID, &name)
		fmt.Printf("GrpID: %s | EnvID: %s | Name: %s\n", id, envID, name)
	}

	// 4. Check Companies
	fmt.Println("\n--- COMPANIES ---")
	rowsComp, _ := db.Query("SELECT id, group_id, name, owner_id FROM companies")
	defer rowsComp.Close()
	for rowsComp.Next() {
		var id, grpID, name string
		var ownerID sql.NullString
		rowsComp.Scan(&id, &grpID, &name, &ownerID)
		owner := "NULL"
		if ownerID.Valid { owner = ownerID.String }
		fmt.Printf("CompID: %s | GrpID: %s | Name: %s | Owner: %s\n", id, grpID, name, owner)
	}
}

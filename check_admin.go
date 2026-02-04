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

	var count int
	err = db.QueryRow("SELECT count(*) FROM environments").Scan(&count)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Total Environments: %d\n", count)

	rows, err := db.Query("SELECT id, name FROM environments")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		rows.Scan(&id, &name)
		fmt.Printf("Env: %s (%s)\n", name, id)
	}
}

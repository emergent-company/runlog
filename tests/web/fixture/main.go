package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/emergent-company/runlog"
)

func main() {
	dbPath := flag.String("db", "", "path to SQLite database")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "usage: fixture --db <path>")
		os.Exit(1)
	}

	db, err := runlog.OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if err := seed(db); err != nil {
		log.Fatalf("seed: %v", err)
	}
	if err := seedDummy(db); err != nil {
		log.Fatalf("seedDummy: %v", err)
	}
	fmt.Println("fixture: seeded", *dbPath)
}

package main

import (
	"flag"
	"log"
	"os"
	"time"

	dbManager "zomeDbManager/db"

	_ "github.com/go-sql-driver/mysql"
)

var logger = log.New(os.Stdout, "dbManager: ", log.LstdFlags|log.Lshortfile)

// 2 week duration
const defaultExpiry = 2 * 7 * 24 * time.Hour

func main() {
	// cli args path
	dbPath := flag.String("dbPath", "", "sets the path to the database file")
	garbageCollect := flag.Bool("garbageCollect", true, "enable garbage collection")
	messageExpiryFlag := flag.String("messageExpiry", defaultExpiry.String(), "sets the expiry date for messages")
	deleteExpiredFiles := flag.Bool("deleteExpiredFiles", true, "delete expired files")
	help := flag.Bool("h", false, "Display Help")
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		return
	}
	var err error

	messageExpiry, err := time.ParseDuration(*messageExpiryFlag)
	if err != nil {
		logger.Println("Error parsing message expiry date")
		logger.Println(err)
		panic(err)
	}

	// check if dbPath is empty
	dbExisted, err := dbManager.CreateDbIfNotExists(*dbPath)
	if err != nil {
		logger.Println("Error creating database file")
		logger.Println(err)
		panic(err)
	}
	tx, err := dbManager.InitTables(*dbPath, !dbExisted)
	if err != nil {
		logger.Println("Error initializing database tables")
		logger.Println(err)
		panic(err)
	}
	logger.Println("Database initialized")

	// start garbage collector
	if *garbageCollect {
		go panic(dbManager.GarbageCollector(tx, messageExpiry, *deleteExpiredFiles))
	}

	// startServer()
}

// TODO: change this to protobufs over a "transport", whether it be tor, meshtastic or http, add sessiontoken auth and encryption
// func startServer() {
// 	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
// 		fmt.Fprintf(w, "Hello World")
// 	})

// 	log.Fatal(http.ListenAndServe(":8080", nil))
// }

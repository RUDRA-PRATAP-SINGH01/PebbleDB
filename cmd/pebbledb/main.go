package main

import (
	"fmt"
	"os"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	dir := envOr("PEBBLEDB_DIR", "./pebbledb-data")
	args := os.Args[1:]
	if args[0] == "-dir" && len(args) >= 2 {
		dir = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	database, err := db.Open(db.Options{Dir: dir})
	if err != nil {
		fatal(err)
	}
	defer database.Close()

	switch args[0] {
	case "put":
		if len(args) != 3 {
			fatal(fmt.Errorf("usage: pebbledb put <key> <value>"))
		}
		if err := database.Put([]byte(args[1]), []byte(args[2])); err != nil {
			fatal(err)
		}
	case "get":
		if len(args) != 2 {
			fatal(fmt.Errorf("usage: pebbledb get <key>"))
		}
		val, err := database.Get([]byte(args[1]))
		if err == db.ErrNotFound {
			os.Exit(1)
		}
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(val))
	case "delete":
		if len(args) != 2 {
			fatal(fmt.Errorf("usage: pebbledb delete <key>"))
		}
		if err := database.Delete([]byte(args[1])); err != nil {
			fatal(err)
		}
	case "scan":
		var start, end []byte
		switch len(args) {
		case 1:
		case 2:
			start = []byte(args[1])
		case 3:
			start = []byte(args[1])
			end = []byte(args[2])
		default:
			fatal(fmt.Errorf("usage: pebbledb scan [start] [end]"))
		}
		it, err := database.Scan(start, end)
		if err != nil {
			fatal(err)
		}
		defer it.Close()
		for it.Valid() {
			fmt.Printf("%s\t%s\n", it.Key(), it.Value())
			if err := it.Next(); err != nil {
				fatal(err)
			}
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `PebbleDB CLI

Usage:
  pebbledb [-dir <path>] put <key> <value>
  pebbledb [-dir <path>] get <key>
  pebbledb [-dir <path>] delete <key>
  pebbledb [-dir <path>] scan [start] [end]

Environment:
  PEBBLEDB_DIR  default database directory (./pebbledb-data)
`)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

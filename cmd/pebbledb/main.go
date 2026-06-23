package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
)

func main() {
	fs := flag.NewFlagSet("pebbledb", flag.ExitOnError)
	dir := fs.String("dir", envOr("PEBBLEDB_DIR", "./pebbledb-data"), "database directory")
	syncWrites := fs.Bool("sync-writes", envBool("PEBBLEDB_SYNC_WRITES"), "fsync WAL before each Put/Delete returns")
	_ = fs.Parse(os.Args[1:])

	args := fs.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	database, err := db.Open(db.Options{
		Dir:        *dir,
		SyncWrites: *syncWrites,
	})
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
	case "sync":
		if len(args) != 1 {
			fatal(fmt.Errorf("usage: pebbledb sync"))
		}
		if err := database.Sync(); err != nil {
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
  pebbledb [flags] put <key> <value>
  pebbledb [flags] get <key>
  pebbledb [flags] delete <key>
  pebbledb [flags] sync
  pebbledb [flags] scan [start] [end]

Flags:
  -dir string
        database directory (default ./pebbledb-data or PEBBLEDB_DIR)
  -sync-writes
        fsync WAL before each Put/Delete returns (default false)

Environment:
  PEBBLEDB_DIR           default database directory
  PEBBLEDB_SYNC_WRITES   set to 1, true, or yes to enable -sync-writes

Durability:
  By default Put/Delete use group commit (async WAL fsync). Call sync after
  writes that must survive crash, or pass -sync-writes for synchronous mode.
`)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

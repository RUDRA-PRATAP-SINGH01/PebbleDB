package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
)

type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string {
	return e.msg
}

type cliConfig struct {
	dir        string
	syncWrites bool
}

func run(args []string, stdout, stderr io.Writer) (err error) {
	cfg, cmdArgs, err := parseCLI(args, stderr)
	if err != nil {
		return err
	}
	if len(cmdArgs) == 0 {
		printUsage(stderr)
		return &exitCodeError{code: 1, msg: "missing command"}
	}

	switch cmdArgs[0] {
	case "help", "-h", "--help":
		printUsage(stderr)
		return nil
	}

	database, err := db.Open(db.Options{
		Dir:        cfg.dir,
		SyncWrites: cfg.syncWrites,
	})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()

	return runCommand(database, cmdArgs, stdout, stderr)
}

func parseCLI(args []string, stderr io.Writer) (cliConfig, []string, error) {
	cfg := cliConfig{
		dir:        envOr("PEBBLEDB_DIR", "./pebbledb-data"),
		syncWrites: envBool("PEBBLEDB_SYNC_WRITES"),
	}

	fs := flag.NewFlagSet("pebbledb", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", cfg.dir, "database directory")
	syncWrites := fs.Bool("sync-writes", cfg.syncWrites, "fsync WAL before each Put/Delete returns")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, nil, err
	}

	cfg.dir = *dir
	cfg.syncWrites = *syncWrites
	return cfg, fs.Args(), nil
}

func runCommand(database *db.DB, args []string, stdout, stderr io.Writer) (err error) {
	switch args[0] {
	case "put":
		if len(args) != 3 {
			return fmt.Errorf("usage: pebbledb put <key> <value>")
		}
		return database.Put([]byte(args[1]), []byte(args[2]))
	case "get":
		if len(args) != 2 {
			return fmt.Errorf("usage: pebbledb get <key>")
		}
		val, getErr := database.Get([]byte(args[1]))
		if errors.Is(getErr, db.ErrNotFound) {
			return &exitCodeError{code: 1, msg: "key not found"}
		}
		if getErr != nil {
			return getErr
		}
		fmt.Fprintln(stdout, string(val))
		return nil
	case "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: pebbledb delete <key>")
		}
		return database.Delete([]byte(args[1]))
	case "sync":
		if len(args) != 1 {
			return fmt.Errorf("usage: pebbledb sync")
		}
		return database.Sync()
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
			return fmt.Errorf("usage: pebbledb scan [start] [end]")
		}
		it, scanErr := database.Scan(start, end)
		if scanErr != nil {
			return scanErr
		}
		defer func() {
			if closeErr := it.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close scan iterator: %w", closeErr))
			}
		}()
		for it.Valid() {
			fmt.Fprintf(stdout, "%s\t%s\n", it.Key(), it.Value())
			if nextErr := it.Next(); nextErr != nil {
				return nextErr
			}
		}
		return nil
	default:
		printUsage(stderr)
		return &exitCodeError{code: 1, msg: fmt.Sprintf("unknown command %q", args[0])}
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `PebbleDB CLI

Usage:
  pebbledb [flags] <command> [arguments]

Commands:
  put <key> <value>    write a key-value pair
  get <key>            read a value (exit 1 if missing)
  delete <key>         tombstone a key
  sync                 fsync pending WAL writes
  scan [start] [end]   iterate keys in [start, end)
  help                 show this help

Flags:
  -dir string
        database directory (default ./pebbledb-data or PEBBLEDB_DIR)
  -sync-writes
        fsync WAL before each Put/Delete returns (default false)

Environment:
  PEBBLEDB_DIR           default database directory
  PEBBLEDB_SYNC_WRITES   set to 1, true, or yes to enable -sync-writes

Durability:
  By default Put/Delete use group commit (async WAL fsync). Run sync after
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

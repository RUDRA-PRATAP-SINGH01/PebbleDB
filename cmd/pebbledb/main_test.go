package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
)

func TestCLIHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Commands:") {
		t.Fatalf("help output missing commands: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCLIFlagHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(stdout.String(), "Commands:") {
			t.Fatalf("%v: help missing from stdout: %q", args, stdout.String())
		}
	}
}

func TestCLIDatabaseLockedMessage(t *testing.T) {
	dir := t.TempDir()
	db1, err := db.Open(db.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db1.Close()
	})

	var stdout, stderr bytes.Buffer
	err = run([]string{"-dir", dir, "get", "k"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected locked error")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("error = %v, want locked message", err)
	}
}

func TestCLIPutGetSync(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer

	if err := run([]string{"-dir", dir, "put", "k", "v"}, &stdout, &stderr); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := run([]string{"-dir", dir, "sync"}, &stdout, &stderr); err != nil {
		t.Fatalf("sync: %v", err)
	}

	stdout.Reset()
	if err := run([]string{"-dir", dir, "get", "k"}, &stdout, &stderr); err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "v" {
		t.Fatalf("got %q, want v", stdout.String())
	}
}

func TestCLIGetNotFoundExitCode(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer

	err := run([]string{"-dir", dir, "get", "missing"}, &stdout, &stderr)
	var exitErr *exitCodeError
	if !errors.As(err, &exitErr) || exitErr.code != 1 {
		t.Fatalf("get missing key = %T %v, want exitCodeError code 1", err, err)
	}
}

func TestCLISyncWritesFlag(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer

	if err := run([]string{"-dir", dir, "-sync-writes", "put", "k", "v"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(db.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := database.Close(); closeErr != nil {
			t.Fatalf("close: %v", closeErr)
		}
	})

	val, err := database.Get([]byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q, want v", val)
	}
}

package db

import "testing"

func TestDBPutGet(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.Put([]byte("name"), []byte("Alice"))
	if err != nil {
		t.Fatal(err)
	}
	val, err := db.Get([]byte("name"))
	if err != nil || string(val) != "Alice" {
		t.Errorf("got %s, want Alice", val)
	}
}

func TestDBDelete(t *testing.T) {
	dir := t.TempDir()
	db, _ := Open(Options{Dir: dir})
	defer db.Close()

	db.Put([]byte("x"), []byte("100"))
	db.Delete([]byte("x"))
	_, err := db.Get([]byte("x"))
	if err != ErrNotFound {
		t.Errorf("expected not found")
	}
}

func TestDBRecovery(t *testing.T) {
	dir := t.TempDir()
	// First open, write data
	db1, _ := Open(Options{Dir: dir})
	db1.Put([]byte("key"), []byte("value"))
	db1.Close()

	// Second open should recover from WAL
	db2, _ := Open(Options{Dir: dir})
	defer db2.Close()
	val, err := db2.Get([]byte("key"))
	if err != nil || string(val) != "value" {
		t.Errorf("recovery failed: %v, %s", err, val)
	}
}
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
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("x"), []byte("100")); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete([]byte("x")); err != nil {
		t.Fatal(err)
	}
	_, err = db.Get([]byte("x"))
	if err != ErrNotFound {
		t.Errorf("expected not found")
	}
}

func TestDBRecovery(t *testing.T) {
	dir := t.TempDir()

	db1, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := db1.Put([]byte("key"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	val, err := db2.Get([]byte("key"))
	if err != nil || string(val) != "value" {
		t.Errorf("recovery failed: %v, %s", err, val)
	}
}
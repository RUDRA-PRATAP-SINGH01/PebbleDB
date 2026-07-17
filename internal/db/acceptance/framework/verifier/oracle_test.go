package verifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

func TestOracleLoaderAcceptsValid(t *testing.T) {
	dir := t.TempDir()
	state := dataset.NewMapExpectedState(1, 1)
	state.ScenarioID = "s1"
	state.ExecutionID = "e1"
	state.State["k"] = types.ValueSnapshot{Value: []byte("v"), Version: 1}
	if err := state.Persist(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewOracleLoader(OracleLoadOptions{
		RequireChecksum:     true,
		ExpectedScenarioID:  "s1",
		ExpectedExecutionID: "e1",
	}).Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Checksum == "" {
		t.Fatal("expected checksum")
	}
}

func TestOracleLoaderRejectsCorruptChecksum(t *testing.T) {
	dir := t.TempDir()
	state := dataset.NewMapExpectedState(1, 1)
	state.State["k"] = types.ValueSnapshot{Value: []byte("v"), Version: 1}
	if err := state.Persist(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, dataset.ExpectedStateFileName)
	var raw map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["checksum"] = json.RawMessage(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatal(err)
	}
	_, err = DefaultOracleLoader().Load(dir)
	if err == nil {
		t.Fatal("expected checksum failure")
	}
}

func TestOracleLoaderRejectsSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, dataset.ExpectedStateFileName)
	doc := `{"schema_version":99,"seed":1,"count":0,"state":{},"checksum":"x"}`
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := DefaultOracleLoader().Load(dir)
	if err == nil {
		t.Fatal("expected schema rejection")
	}
}

func TestOracleLoaderRejectsScenarioMismatch(t *testing.T) {
	dir := t.TempDir()
	state := dataset.NewMapExpectedState(1, 0)
	state.ScenarioID = "a"
	state.State = map[string]types.ValueSnapshot{}
	if err := state.Persist(dir); err != nil {
		t.Fatal(err)
	}
	_, err := NewOracleLoader(OracleLoadOptions{
		RequireChecksum:    true,
		ExpectedScenarioID: "b",
	}).Load(dir)
	if err == nil {
		t.Fatal("expected scenario mismatch")
	}
}

func TestOracleLoaderMissingFile(t *testing.T) {
	_, err := DefaultOracleLoader().Load(t.TempDir())
	if err == nil {
		t.Fatal("expected missing oracle error")
	}
}

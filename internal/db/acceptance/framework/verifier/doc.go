// Package verifier implements the ATF Verification Engine: an orchestrator that
// loads the oracle (expected_state.json), opens a recovered PebbleDB directory,
// and runs independent verifier modules to determine post-crash correctness.
//
// The package consumes execution artifacts only. It does not know how scenarios
// are scheduled or how child processes write data.
package verifier

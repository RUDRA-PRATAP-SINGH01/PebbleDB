// Package errors defines the typed validation, state, registration, and runtime
// errors of the PebbleDB Acceptance Testing Framework (ATF).
//
// Dependency Rules:
// - This is a leaf package. It must not import any other internal framework packages.
package errors

import (
	"fmt"
)

// ErrorCode identifies the logical category of the framework error.
type ErrorCode string

const (
	ErrUnknown            ErrorCode = "ERR_UNKNOWN"
	ErrConfiguration      ErrorCode = "ERR_CONFIGURATION"
	ErrRegistration       ErrorCode = "ERR_REGISTRATION"
	ErrValidation         ErrorCode = "ERR_VALIDATION"
	ErrExecution          ErrorCode = "ERR_EXECUTION"
	ErrStateTransition    ErrorCode = "ERR_STATE_TRANSITION"
	ErrResourceExhausted  ErrorCode = "ERR_RESOURCE_EXHAUSTED"
	ErrLockAcquisition    ErrorCode = "ERR_LOCK_ACQUISITION"
	ErrManifestParse      ErrorCode = "ERR_MANIFEST_PARSE"
	ErrWalParse           ErrorCode = "ERR_WAL_PARSE"
	ErrInvariantViolation ErrorCode = "ERR_INVARIANT_VIOLATION"
)

// FrameworkError represents any framework-level error.
type FrameworkError struct {
	Code    ErrorCode
	Message string
	Err     error
}

// Error formats the framework error message.
func (e *FrameworkError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error if present.
func (e *FrameworkError) Unwrap() error {
	return e.Err
}

// NewConfigurationError creates a new configuration validation/loading error.
func NewConfigurationError(msg string, err error) error {
	return &FrameworkError{
		Code:    ErrConfiguration,
		Message: msg,
		Err:     err,
	}
}

// NewRegistrationError creates a new scenario or plugin registration error.
func NewRegistrationError(msg string, err error) error {
	return &FrameworkError{
		Code:    ErrRegistration,
		Message: msg,
		Err:     err,
	}
}

// NewValidationError creates an expected state or validator failure error.
func NewValidationError(msg string, err error) error {
	return &FrameworkError{
		Code:    ErrValidation,
		Message: msg,
		Err:     err,
	}
}

// NewExecutionError creates a subprocess execution error.
func NewExecutionError(msg string, err error) error {
	return &FrameworkError{
		Code:    ErrExecution,
		Message: msg,
		Err:     err,
	}
}

// NewStateError creates a session state transition error.
func NewStateError(msg string, err error) error {
	return &FrameworkError{
		Code:    ErrStateTransition,
		Message: msg,
		Err:     err,
	}
}

// NewResourceError creates a resource exhaustion or allocation error.
func NewResourceError(msg string, err error) error {
	return &FrameworkError{
		Code:    ErrResourceExhausted,
		Message: msg,
		Err:     err,
	}
}

// NewLockError creates an exclusive directory lock error.
func NewLockError(msg string, err error) error {
	return &FrameworkError{
		Code:    ErrLockAcquisition,
		Message: msg,
		Err:     err,
	}
}

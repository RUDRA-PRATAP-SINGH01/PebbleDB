package crash

import "fmt"

// ErrorCode classifies crash-framework failures.
type ErrorCode string

const (
	// ErrInvalidConfig indicates configuration failed validation.
	ErrInvalidConfig ErrorCode = "ERR_CRASH_INVALID_CONFIG"
	// ErrUnknownPoint indicates the crash point ID is not registered.
	ErrUnknownPoint ErrorCode = "ERR_CRASH_UNKNOWN_POINT"
	// ErrDuplicatePoint indicates a crash point ID is already registered.
	ErrDuplicatePoint ErrorCode = "ERR_CRASH_DUPLICATE_POINT"
	// ErrDependency indicates a crash point dependency is missing or invalid.
	ErrDependency ErrorCode = "ERR_CRASH_DEPENDENCY"
	// ErrPolicyRejected indicates the active policy forbids crashing.
	ErrPolicyRejected ErrorCode = "ERR_CRASH_POLICY_REJECTED"
	// ErrPolicyUnsupported indicates a policy kind is not implemented yet.
	ErrPolicyUnsupported ErrorCode = "ERR_CRASH_POLICY_UNSUPPORTED"
	// ErrHookRejected indicates the hook cannot run in the given context.
	ErrHookRejected ErrorCode = "ERR_CRASH_HOOK_REJECTED"
	// ErrHookExecute indicates CrashHook.Execute failed.
	ErrHookExecute ErrorCode = "ERR_CRASH_HOOK_EXECUTE"
)

// Error is a typed crash-framework error.
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error, if any.
func (e *Error) Unwrap() error { return e.Err }

func newError(code ErrorCode, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}

package apperror

import "fmt"

// AppError is a structured application error with a code for sentinel matching.
type AppError struct {
	Code    string
	Message string
	Cause   error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

// Is compares Code for equality so errors.Is works with Wrap'd errors.
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// Wrap clones a sentinel and attaches a cause.
func Wrap(sentinel *AppError, cause error) *AppError {
	return &AppError{
		Code:    sentinel.Code,
		Message: sentinel.Message,
		Cause:   cause,
	}
}

// Sentinel errors.
var (
	ErrNotFound     = &AppError{Code: "NOT_FOUND", Message: "not found"}
	ErrDuplicate    = &AppError{Code: "DUPLICATE", Message: "already exists"}
	ErrInvalidInput = &AppError{Code: "INVALID_INPUT", Message: "invalid input"}
	ErrDatabase     = &AppError{Code: "DATABASE", Message: "database error"}
)

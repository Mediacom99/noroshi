package apperror

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapPreservesCode(t *testing.T) {
	cause := fmt.Errorf("sql: no rows")
	wrapped := Wrap(ErrNotFound, cause)

	if !errors.Is(wrapped, ErrNotFound) {
		t.Fatal("errors.Is(Wrap(ErrNotFound, cause), ErrNotFound) should be true")
	}
}

func TestWrapDifferentSentinels(t *testing.T) {
	wrapped := Wrap(ErrNotFound, fmt.Errorf("some error"))

	if errors.Is(wrapped, ErrDuplicate) {
		t.Fatal("errors.Is(Wrap(ErrNotFound, ...), ErrDuplicate) should be false")
	}
}

func TestUnwrapReturnsCause(t *testing.T) {
	cause := fmt.Errorf("underlying")
	wrapped := Wrap(ErrDatabase, cause)

	if wrapped.Unwrap() != cause {
		t.Fatal("Unwrap should return the original cause")
	}
}

func TestErrorWithCause(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	wrapped := Wrap(ErrDatabase, cause)

	want := "database error: connection refused"
	if got := wrapped.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestErrorWithoutCause(t *testing.T) {
	if got := ErrNotFound.Error(); got != "not found" {
		t.Fatalf("Error() = %q, want %q", got, "not found")
	}
}

func TestIsWithNonAppError(t *testing.T) {
	wrapped := Wrap(ErrNotFound, fmt.Errorf("cause"))

	if errors.Is(wrapped, fmt.Errorf("not found")) {
		t.Fatal("errors.Is should return false for non-AppError target")
	}
}

func TestSentinelIsItself(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatal("sentinel should match itself")
	}
}

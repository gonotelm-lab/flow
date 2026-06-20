package errors

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
)

func TestNewAndExtract(t *testing.T) {
	const key DomainErrorKey = "NAMESPACE_NOT_FOUND"
	const code = codes.NotFound

	err := New(code, key)
	if err == nil {
		t.Fatalf("expected non-nil error")
	}

	domainErr, ok := As(err)
	if !ok {
		t.Fatalf("expected domain error to be extracted")
	}
	if domainErr.Code() != code {
		t.Fatalf("unexpected code, got=%v want=%v", domainErr.Code(), code)
	}
	if domainErr.Key() != key {
		t.Fatalf("unexpected key, got=%q want=%q", domainErr.Key(), key)
	}

	gotKey, ok := Extract(err)
	if !ok {
		t.Fatalf("expected key to be extracted")
	}
	if gotKey != key {
		t.Fatalf("unexpected key, got=%q want=%q", gotKey, key)
	}
}

func TestWrapAndExtract(t *testing.T) {
	const key DomainErrorKey = "NAMESPACE_ALREADY_EXISTS"
	const code = codes.AlreadyExists

	cause := errors.New("duplicate key value")
	err := Wrap(cause, code, key)
	if err == nil {
		t.Fatalf("expected non-nil error")
	}

	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped error to keep original cause")
	}

	gotKey, ok := Extract(err)
	if !ok {
		t.Fatalf("expected key to be extracted")
	}
	if gotKey != key {
		t.Fatalf("unexpected key, got=%q want=%q", gotKey, key)
	}

	domainErr, ok := As(err)
	if !ok {
		t.Fatalf("expected domain error to be extracted")
	}
	if domainErr.Code() != code {
		t.Fatalf("unexpected code, got=%v want=%v", domainErr.Code(), code)
	}
}

func TestExtractWithStandardWrapChain(t *testing.T) {
	const key DomainErrorKey = "NAMESPACE_REQUIRED"
	const code = codes.InvalidArgument

	err := fmt.Errorf("service layer: %w", Wrap(errors.New("invalid payload"), code, key))
	gotKey, ok := Extract(err)
	if !ok {
		t.Fatalf("expected key to be extracted from std wrap chain")
	}
	if gotKey != key {
		t.Fatalf("unexpected key, got=%q want=%q", gotKey, key)
	}
}

func TestExtractWithPkgErrorsWrapChain(t *testing.T) {
	const key DomainErrorKey = "NAMESPACE_NAME_REQUIRED"
	const code = codes.InvalidArgument

	base := Wrap(errors.New("empty name"), code, key)
	err := errors.WithStack(errors.WithMessage(base, "repository create failed"))

	gotKey, ok := Extract(err)
	if !ok {
		t.Fatalf("expected key to be extracted from github.com/pkg/errors chain")
	}
	if gotKey != key {
		t.Fatalf("unexpected key, got=%q want=%q", gotKey, key)
	}
}

func TestExtractNilAndUnknownError(t *testing.T) {
	if _, ok := As(nil); ok {
		t.Fatalf("expected nil error to return ok=false")
	}

	if _, ok := Extract(nil); ok {
		t.Fatalf("expected nil error to return ok=false")
	}

	if _, ok := As(errors.New("plain error")); ok {
		t.Fatalf("expected plain error to return ok=false")
	}

	if _, ok := Extract(errors.New("plain error")); ok {
		t.Fatalf("expected plain error to return ok=false")
	}
}

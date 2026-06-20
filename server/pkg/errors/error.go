package errors

import (
	stderr "errors"

	"google.golang.org/grpc/codes"
)

type DomainErrorKey string

type DomainError struct {
	code    codes.Code
	key     DomainErrorKey
	cause   error
	details []string
}

func (e *DomainError) Error() string {
	return string(e.key)
}

func (e *DomainError) Code() codes.Code {
	return e.code
}

func (e *DomainError) Key() DomainErrorKey {
	return e.key
}

func (e *DomainError) Unwrap() error {
	return e.cause
}

func (e *DomainError) Details() []string {
	return e.details
}

func (e *DomainError) Duplicate() *DomainError {
	newE := *e
	newE.details = make([]string, len(e.details))
	copy(newE.details, e.details)

	return &newE
}

func (e *DomainError) WithDetail(s string) *DomainError {
	newE := e.Duplicate()
	newE.details = append(newE.details, s)

	return newE
}

func New(code codes.Code, key DomainErrorKey) *DomainError {
	return &DomainError{
		code: code,
		key:  key,
	}
}

func Wrap(cause error, code codes.Code, key DomainErrorKey) *DomainError {
	return &DomainError{
		code:  code,
		key:   key,
		cause: cause,
	}
}

func As(err error) (*DomainError, bool) {
	if err == nil {
		return nil, false
	}

	var domainErr *DomainError
	if stderr.As(err, &domainErr) && domainErr != nil && domainErr.key != "" {
		return domainErr, true
	}

	type causer interface {
		Cause() error
	}

	seen := map[error]struct{}{}
	for current := err; current != nil; {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}

		if stderr.As(current, &domainErr) && domainErr != nil && domainErr.key != "" {
			return domainErr, true
		}

		causeCarrier, ok := current.(causer)
		if !ok {
			break
		}

		next := causeCarrier.Cause()
		if next == nil || next == current {
			break
		}

		current = next
	}

	return nil, false
}

func Extract(err error) (DomainErrorKey, bool) {
	domainErr, ok := As(err)
	if !ok {
		return "", false
	}

	return domainErr.key, true
}

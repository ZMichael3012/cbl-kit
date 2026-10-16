// Package grpcmw contains the error types and gRPC interceptors of the CBL platform.
package grpcmw

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Kind describes the class of a domain error for mapping to a gRPC code and an HTTP status.
type Kind uint8

// Platform error classes.
const (
	// KindInternal describes an internal error with no details exposed outward.
	KindInternal Kind = iota
	// KindNotFound describes a missing entity.
	KindNotFound
	// KindInvalidArgument describes an invalid request.
	KindInvalidArgument
	// KindAlreadyExists describes a creation conflict.
	KindAlreadyExists
	// KindFailedPrecondition describes an invalid entity state.
	KindFailedPrecondition
	// KindResourceExhausted describes a limit excess.
	KindResourceExhausted
	// KindUnavailable describes dependency unavailability.
	KindUnavailable
)

// String returns the text name of the error class.
func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "not_found"
	case KindInvalidArgument:
		return "invalid_argument"
	case KindAlreadyExists:
		return "already_exists"
	case KindFailedPrecondition:
		return "failed_precondition"
	case KindResourceExhausted:
		return "resource_exhausted"
	case KindUnavailable:
		return "unavailable"
	default:
		return "internal"
	}
}

// Error describes a domain error with a class, a code and a message.
type Error struct {
	// Kind sets the error class.
	Kind Kind
	// Code sets the machine-readable code, for example payment_not_found.
	Code string
	// Message sets the human-readable message.
	Message string
	// Err sets the source error for wrapping.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap returns the source error for errors.Is and errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// E creates a domain error with a class, a code and a message.
func E(kind Kind, code, message string, err error) *Error {
	return &Error{Kind: kind, Code: code, Message: message, Err: err}
}

// KindOf returns the error class; unknown errors map to KindInternal.
func KindOf(err error) Kind {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind
	}
	return KindInternal
}

// CodeOf returns the machine-readable error code or an empty string.
func CodeOf(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

// MapError converts an error into a gRPC status.
// Ready statuses are passed through as is, domain errors are mapped by class,
// and unknown errors are hidden behind internal error without leaking details.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	if s, ok := status.FromError(err); ok && s.Code() != codes.Unknown {
		return err
	}
	var target *Error
	if !errors.As(err, &target) {
		return status.Error(codes.Internal, "internal error")
	}
	message := target.Error()
	if target.Kind == KindInternal {
		message = "internal error"
	}
	return status.Error(mapKind(target.Kind), message)
}

// mapKind translates an error class into a gRPC code.
func mapKind(k Kind) codes.Code {
	switch k {
	case KindNotFound:
		return codes.NotFound
	case KindInvalidArgument:
		return codes.InvalidArgument
	case KindAlreadyExists:
		return codes.AlreadyExists
	case KindFailedPrecondition:
		return codes.FailedPrecondition
	case KindResourceExhausted:
		return codes.ResourceExhausted
	case KindUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

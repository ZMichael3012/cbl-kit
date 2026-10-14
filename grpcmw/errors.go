// Пакет grpcmw содержит типы ошибок и gRPC-интерсепторы платформы CBL.
package grpcmw

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Kind описывает класс доменной ошибки для маппинга в gRPC-код и HTTP-статус.
type Kind uint8

// Классы ошибок платформы.
const (
	// KindInternal описывает внутреннюю ошибку без деталей наружу.
	KindInternal Kind = iota
	// KindNotFound описывает отсутствующую сущность.
	KindNotFound
	// KindInvalidArgument описывает некорректный запрос.
	KindInvalidArgument
	// KindAlreadyExists описывает конфликт создания.
	KindAlreadyExists
	// KindFailedPrecondition описывает недопустимое состояние сущности.
	KindFailedPrecondition
	// KindResourceExhausted описывает превышение лимита.
	KindResourceExhausted
	// KindUnavailable описывает недоступность зависимости.
	KindUnavailable
)

// String возвращает текстовое имя класса ошибки.
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

// Error описывает доменную ошибку с классом, кодом и сообщением.
type Error struct {
	// Kind задает класс ошибки.
	Kind Kind
	// Code задает машиночитаемый код, например payment_not_found.
	Code string
	// Message задает человекочитаемое сообщение.
	Message string
	// Err задает исходную ошибку для оборачивания.
	Err error
}

// Error реализует интерфейс error.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap возвращает исходную ошибку для errors.Is и errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// E создает доменную ошибку с классом, кодом и сообщением.
func E(kind Kind, code, message string, err error) *Error {
	return &Error{Kind: kind, Code: code, Message: message, Err: err}
}

// KindOf возвращает класс ошибки; неизвестные ошибки относятся к KindInternal.
func KindOf(err error) Kind {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind
	}
	return KindInternal
}

// CodeOf возвращает машиночитаемый код ошибки или пустую строку.
func CodeOf(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

// MapError преобразует ошибку в gRPC-статус.
// Готовые статусы передаются как есть, доменные ошибки маппятся по классу,
// неизвестные ошибки скрываются за internal error без утечки деталей.
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

// mapKind переводит класс ошибки в gRPC-код.
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

package param

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable machine-readable code for a parameter parse failure.
type ErrorCode string

const (
	// CodeInvalidBody reports that the request body could not be decoded as a
	// JSON object.
	CodeInvalidBody ErrorCode = "invalid_request_body"

	// CodeMissing reports that a required parameter was absent or treated as
	// absent by its null policy.
	CodeMissing ErrorCode = "missing_request_parameter"

	// CodeRequiredChoice reports that a request omitted every parameter in a
	// required choice group.
	CodeRequiredChoice ErrorCode = "request_parameter_choice_required"

	// CodeMutuallyExclusive reports that a request supplied too many parameters
	// from a mutually exclusive group.
	CodeMutuallyExclusive ErrorCode = "request_parameters_mutually_exclusive"

	// CodeUnexpected reports that the caller supplied a parameter outside the
	// visible request contract. Restricted parameters use this same code so the
	// API does not reveal hidden parameters.
	CodeUnexpected ErrorCode = "unexpected_request_parameter"

	// CodeNullRejected reports that a present parameter was null but its null
	// policy rejects null.
	CodeNullRejected ErrorCode = "request_parameter_null_rejected"

	// CodeTypeMismatch reports that a JSON value did not match the declared
	// wire shape.
	CodeTypeMismatch ErrorCode = "request_parameter_type_mismatch"

	// CodeValueNotAllowed reports that a parameter value is outside the
	// declared boundary choices, such as a string enum or discriminator value.
	CodeValueNotAllowed ErrorCode = "request_parameter_value_not_allowed"

	// CodeTooSmall reports that the measured parameter size is below its lower
	// bound.
	CodeTooSmall ErrorCode = "request_parameter_too_small"

	// CodeTooLarge reports that the measured parameter size is above its upper
	// bound.
	CodeTooLarge ErrorCode = "request_parameter_too_large"

	// CodeParseFailed reports that a parser could not convert an otherwise
	// acceptable wire value into the endpoint's domain value.
	CodeParseFailed ErrorCode = "request_parameter_parse_failed"

	// CodeInvalid reports that a parser rejected a value as invalid.
	CodeInvalid ErrorCode = "invalid_request_parameter"
)

// Error describes one request-parameter failure.
//
// Error is the native parse error returned by this package. It intentionally
// contains only caller-safe response fields plus the underlying cause for logs.
type Error struct {
	// Param is the request parameter path that failed, such as
	// "customer_data.email_address".
	Param string

	// Code is the stable machine-readable failure code.
	Code ErrorCode

	// Message is safe to return to API callers.
	Message string

	// Cause is the underlying parser or decoder error, when one exists.
	Cause error
}

// Error formats a compact diagnostic string for logs and tests.
func (err *Error) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", err.Param, err.Message, err.Cause)
	}
	return fmt.Sprintf("%s: %s", err.Param, err.Message)
}

// Unwrap returns the parser or decoder error that caused this parameter error.
func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// NewError returns a parameter error with a stable code and caller-safe message.
func NewError(param string, code ErrorCode, message string, cause error) *Error {
	return paramError(param, code, message, cause)
}

// Invalid returns a CodeInvalid error for a parser-rejected parameter.
func Invalid(param string, message string) *Error {
	return NewError(param, CodeInvalid, message, nil)
}

func paramError(param string, code ErrorCode, message string, cause error) *Error {
	if existing, ok := errors.AsType[*Error](cause); ok {
		return existing
	}
	return &Error{
		Param:   param,
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

func parserError(param string, err error) *Error {
	return paramError(
		param,
		CodeParseFailed,
		fmt.Sprintf("`%s` could not be parsed", param),
		err,
	)
}

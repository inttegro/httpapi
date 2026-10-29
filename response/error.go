package response

import (
	"net/http"

	e "github.com/inttegro/httpapi/erreur"
	"github.com/inttegro/httpapi/param"
)

// ErrorBody is the standard JSON response body for structured httpapi errors.
type ErrorBody struct {
	// Err is the structured error returned to the client.
	Err *e.Error `json:"error"`
}

// ErrRes is kept as a short compatibility alias for ErrorBody.
type ErrRes = ErrorBody

// ErrorShape describes the standard structured httpapi error response body.
var ErrorShape = Object[ErrorBody](
	Required("error", ErrorObjectShape, func(body ErrorBody) *e.Error {
		return body.Err
	}),
)

// ErrorObjectShape describes the structured error object under ErrorBody.Err.
var ErrorObjectShape = Object[*e.Error](
	Optional("message", String(), func(err *e.Error) (string, bool) {
		return errorString(err, func(err *e.Error) string { return err.Message })
	}),
	Optional("fix_code", String(), func(err *e.Error) (string, bool) {
		return errorString(err, func(err *e.Error) string { return err.FixCode })
	}),
	Optional("detail", String(), func(err *e.Error) (string, bool) {
		return errorString(err, func(err *e.Error) string { return err.Detail })
	}),
	Optional("cause", String(), func(err *e.Error) (string, bool) {
		return errorString(err, func(err *e.Error) string { return err.Cause })
	}),
	Required("type", String(), func(err *e.Error) string {
		return requiredErrorString(err, func(err *e.Error) string { return err.Type })
	}),
	Required("code", String(), func(err *e.Error) string {
		return requiredErrorString(err, func(err *e.Error) string { return err.Code })
	}),
	Required("url", String(), func(err *e.Error) string {
		return requiredErrorString(err, func(err *e.Error) string { return err.URL })
	}),
)

// RenderErr sets a structured error response on the target.
//
// The response status is derived from the error. When the error has no URL,
// RenderErr asks the erreur package URL builder for one.
func RenderErr(r Target, err *e.Error) {
	Render(r, Error(err))
}

// Error returns a structured error response.
//
// The response status is derived from the error. When the error has no URL,
// Error asks the erreur package URL builder for one.
func Error(err *e.Error, options ...Option) *Res {
	if err == nil {
		err = e.Unexpected()
	}

	status := e.ResponseStatus(err)
	bodyErr := *err
	bodyErr.Status = status
	if bodyErr.URL == "" && bodyErr.Code != "" {
		bodyErr.URL = e.URLFor(
			bodyErr.Code,
			bodyErr.Type,
			bodyErr.Cause,
			bodyErr.FixCode,
		)
	}

	return JSON(status, ErrorBody{Err: &bodyErr}, options...)
}

// RenderParamErr converts an ErrInvalidParam into the standard structured
// error response format and sets it on the target.
func RenderParamErr(r Target, err *e.ErrInvalidParam) {
	Render(r, InvalidParamError(err))
}

// InvalidParamError converts an ErrInvalidParam into the standard structured
// error response format.
func InvalidParamError(err *e.ErrInvalidParam, options ...Option) *Res {
	if err == nil {
		return Error(nil, options...)
	}

	status := err.Status
	if status == 0 {
		status = http.StatusBadRequest
	}
	code := err.Code
	if code == "" {
		code = "invalid_request_parameter"
	}
	cause := err.Cause
	if cause == "" {
		cause = e.CauseInvalidParam
	}
	typ := err.Type
	if typ == "" {
		typ = e.TypeInvalidParam
	}
	fixCode := err.FixCode
	if fixCode == "" {
		fixCode = e.FixCodeChangeParams
	}

	return Error(&e.Error{
		Message: err.Mesg,
		FixCode: fixCode,
		Status:  status,
		Cause:   cause,
		Type:    typ,
		Code:    code,
		URL:     e.URLFor(code, typ, cause, fixCode),
	}, options...)
}

// RenderParamError converts a native param parse error into the standard
// structured error response format and sets it on the target.
func RenderParamError(r Target, err *param.Error) {
	Render(r, ParamError(err))
}

// ParamError converts a native param parse error into the standard structured
// error response format.
func ParamError(err *param.Error, options ...Option) *Res {
	return Error(ErrorFromParam(err), options...)
}

// ErrorFromParam converts a native param parse error into the standard
// structured error model.
func ErrorFromParam(err *param.Error) *e.Error {
	if err == nil {
		return e.Unexpected()
	}

	status := http.StatusBadRequest
	code := string(err.Code)
	if code == "" {
		code = string(param.CodeInvalid)
	}
	cause := e.CauseInvalidParam
	typ := e.TypeInvalidParam
	if err.Code == param.CodeInvalidBody {
		cause = e.CauseInvalidBody
		typ = e.TypeInvalidRequest
	}
	if err.Code == param.CodeMissing || err.Code == param.CodeRequiredChoice {
		cause = e.CauseMissingParam
	}

	return &e.Error{
		Message: err.Message,
		FixCode: e.FixCodeChangeParams,
		Detail:  err.Param,
		Status:  status,
		Cause:   cause,
		Type:    typ,
		Code:    code,
		URL:     e.URLFor(code, typ, cause, e.FixCodeChangeParams),
	}
}

func errorString(err *e.Error, getter func(*e.Error) string) (string, bool) {
	value := requiredErrorString(err, getter)
	return value, value != ""
}

func requiredErrorString(err *e.Error, getter func(*e.Error) string) string {
	if err == nil || getter == nil {
		return ""
	}
	return getter(err)
}

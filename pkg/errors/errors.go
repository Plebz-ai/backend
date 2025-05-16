package errors

import (
	"fmt"
	"net/http"
	"runtime/debug"
)

// AppError represents an application error with HTTP status code and error code
type AppError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    any    `json:"details,omitempty"`
	Stack      string `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// WithDetails adds details to the error
func (e *AppError) WithDetails(details any) *AppError {
	e.Details = details
	return e
}

// NewError creates a new application error
func NewError(statusCode int, code string, message string) *AppError {
	return &AppError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Stack:      string(debug.Stack()),
	}
}

// NewBadRequestError creates a 400 Bad Request error
func NewBadRequestError(code string, message string) *AppError {
	return NewError(http.StatusBadRequest, code, message)
}

// NewUnauthorizedError creates a 401 Unauthorized error
func NewUnauthorizedError(code string, message string) *AppError {
	return NewError(http.StatusUnauthorized, code, message)
}

// NewForbiddenError creates a 403 Forbidden error
func NewForbiddenError(code string, message string) *AppError {
	return NewError(http.StatusForbidden, code, message)
}

// NewNotFoundError creates a 404 Not Found error
func NewNotFoundError(code string, message string) *AppError {
	return NewError(http.StatusNotFound, code, message)
}

// NewConflictError creates a 409 Conflict error
func NewConflictError(code string, message string) *AppError {
	return NewError(http.StatusConflict, code, message)
}

// NewInternalServerError creates a 500 Internal Server Error
func NewInternalServerError(code string, message string) *AppError {
	return NewError(http.StatusInternalServerError, code, message)
}

// Is checks if the target error is of type AppError
func Is(err error, target *AppError) bool {
	appErr, ok := err.(*AppError)
	if !ok {
		return false
	}
	return appErr.Code == target.Code
}

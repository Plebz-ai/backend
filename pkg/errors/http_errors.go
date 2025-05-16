package errors

import (
	"fmt"
	"net/http"
)

// ErrorWithDetails adds details to an error and converts it to AppError if it's not already one
func ErrorWithDetails(err error, details any) *AppError {
	if err == nil {
		return nil
	}

	if appErr, ok := err.(*AppError); ok {
		appErr.Details = details
		return appErr
	}

	// Convert standard error to AppError with details
	return &AppError{
		StatusCode: http.StatusInternalServerError,
		Code:       "INTERNAL_ERROR",
		Message:    err.Error(),
		Details:    details,
		Stack:      "", // Stack trace not available for external errors
	}
}

// BadRequestWithDetails creates a 400 Bad Request error with details
func BadRequestWithDetails(code string, message string, details any) *AppError {
	appErr := NewBadRequestError(code, message)
	appErr.Details = details
	return appErr
}

// UnauthorizedWithDetails creates a 401 Unauthorized error with details
func UnauthorizedWithDetails(code string, message string, details any) *AppError {
	appErr := NewUnauthorizedError(code, message)
	appErr.Details = details
	return appErr
}

// ForbiddenWithDetails creates a 403 Forbidden error with details
func ForbiddenWithDetails(code string, message string, details any) *AppError {
	appErr := NewForbiddenError(code, message)
	appErr.Details = details
	return appErr
}

// NotFoundWithDetails creates a 404 Not Found error with details
func NotFoundWithDetails(code string, message string, details any) *AppError {
	appErr := NewNotFoundError(code, message)
	appErr.Details = details
	return appErr
}

// ConflictWithDetails creates a 409 Conflict error with details
func ConflictWithDetails(code string, message string, details any) *AppError {
	appErr := NewConflictError(code, message)
	appErr.Details = details
	return appErr
}

// InternalServerWithDetails creates a 500 Internal Server Error with details
func InternalServerWithDetails(code string, message string, details any) *AppError {
	appErr := NewInternalServerError(code, message)
	appErr.Details = details
	return appErr
}

// FromError converts a standard error to an AppError
// If the error is already an AppError, it is returned as-is
// Otherwise, it is wrapped as an internal server error
func FromError(err error) *AppError {
	if err == nil {
		return nil
	}

	if appErr, ok := err.(*AppError); ok {
		return appErr
	}

	return NewInternalServerError(
		"INTERNAL_ERROR",
		fmt.Sprintf("An unexpected error occurred: %s", err.Error()),
	)
}

// GetStatusCode extracts the HTTP status code from an AppError, returns 500 if not an AppError
func GetStatusCode(err error) int {
	if appErr, ok := err.(*AppError); ok {
		return appErr.StatusCode
	}
	return http.StatusInternalServerError
}

// GetErrorCode extracts the error code from an AppError, returns "UNKNOWN_ERROR" if not an AppError
func GetErrorCode(err error) string {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code
	}
	return "UNKNOWN_ERROR"
}

// GetErrorMessage extracts the error message, returns original error message if not an AppError
func GetErrorMessage(err error) string {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Message
	}
	return err.Error()
}

// GetErrorDetails extracts the details from an AppError, returns nil if not an AppError
func GetErrorDetails(err error) any {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Details
	}
	return nil
}

// HasDetails checks if an AppError has details
func HasDetails(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Details != nil
	}
	return false
}

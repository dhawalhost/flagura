package domain

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel domain errors for errors.Is checking across all layers.
var (
	ErrNotFound              = errors.New("resource not found")
	ErrUnauthorized          = errors.New("unauthorized: missing or invalid credentials")
	ErrForbidden             = errors.New("forbidden: insufficient permissions")
	ErrConflict              = errors.New("resource conflict: entity already exists")
	ErrInvalidInput          = errors.New("invalid input: validation failed")
	ErrUserNotFound          = errors.New("user not found")
	ErrEmailAlreadyExists    = errors.New("an account with this email address already exists")
	ErrOrgNotFound           = errors.New("organization not found")
	ErrProjectNotFound       = errors.New("project not found")
	ErrFlagNotFound          = errors.New("feature flag not found")
	ErrInvalidEnvironment    = errors.New("invalid environment")
	ErrEnvironmentRestricted = errors.New("api key environment restriction violated")
	ErrKeyNotFound           = errors.New("api key not found")
	ErrKeyRevoked            = errors.New("api key revoked")
	ErrFourEyesSelfApproval  = errors.New("author cannot review or approve their own change request")
	ErrInternal              = errors.New("internal server error")
)

// ErrorCode is a structured integer identifying the exact fault layer and condition.
type ErrorCode int

const (
	// -------------------------------------------------------------
	// 1000s: Authentication, Authorization & Security Layer
	// -------------------------------------------------------------
	ErrCodeUnauthorized          ErrorCode = 1001 // Missing or invalid session/bearer token
	ErrCodeForbidden             ErrorCode = 1002 // Insufficient RBAC role
	ErrCodeInvalidCredentials    ErrorCode = 1003 // Incorrect email/password
	ErrCodeAPIKeyNotFound        ErrorCode = 1004 // API key hash not found
	ErrCodeAPIKeyRevoked         ErrorCode = 1005 // API key has been revoked
	ErrCodeEnvironmentRestricted ErrorCode = 1006 // API key environment restriction violated
	ErrCodePasswordTooWeak       ErrorCode = 1007 // Password complexity failure
	ErrCodeEmailAlreadyExists    ErrorCode = 1008 // Duplicate email on signup
	ErrCodeUserNotFound          ErrorCode = 1009 // User/account not found

	// -------------------------------------------------------------
	// 2000s: Multi-Tenancy, Organization & Project Layer
	// -------------------------------------------------------------
	ErrCodeOrgNotFound         ErrorCode = 2001 // Organization not found
	ErrCodeOrgConflict         ErrorCode = 2002 // Organization slug already exists
	ErrCodeProjectNotFound     ErrorCode = 2003 // Project not found
	ErrCodeProjectConflict     ErrorCode = 2004 // Project slug already exists
	ErrCodeProjectAccessDenied ErrorCode = 2005 // User not authorized for this project

	// -------------------------------------------------------------
	// 3000s: Feature Flags & Evaluation Engine Layer
	// -------------------------------------------------------------
	ErrCodeFlagNotFound         ErrorCode = 3001 // Feature flag key/ID not found
	ErrCodeFlagAlreadyExists    ErrorCode = 3002 // Feature flag key conflict
	ErrCodeInvalidEnvironment   ErrorCode = 3003 // Unknown environment target
	ErrCodeInvalidRollout       ErrorCode = 3004 // Percentage not in [0, 100] range
	ErrCodeEvaluationMissingCtx ErrorCode = 3005 // Missing evaluation context (e.g. user_id)
	ErrCodeNotFound             ErrorCode = 3006 // Generic not found

	// -------------------------------------------------------------
	// 4000s: Governance & 4-Eyes Change Approval Layer
	// -------------------------------------------------------------
	ErrCodeChangeRequestNotFound ErrorCode = 4001 // Change request not found
	ErrCodeFourEyesSelfApproval  ErrorCode = 4002 // Author cannot approve own change request
	ErrCodeChangeRequestReviewed ErrorCode = 4003 // Change request already reviewed
	ErrCodeExperimentNotFound    ErrorCode = 4004 // Experiment not found

	// -------------------------------------------------------------
	// 5000s: Storage & Database Layer
	// -------------------------------------------------------------
	ErrCodeDatabaseConnection ErrorCode = 5001 // Postgres pool/connection error
	ErrCodeDatabaseQuery      ErrorCode = 5002 // SQL query execution error
	ErrCodeDatabaseConstraint ErrorCode = 5003 // Unique/foreign key constraint violated

	// -------------------------------------------------------------
	// 6000s: Transport, SDK & Network Layer
	// -------------------------------------------------------------
	ErrCodeSSEStreamDisconnect ErrorCode = 6001 // Client disconnected from stream
	ErrCodeCircuitBreakerOpen  ErrorCode = 6002 // Fast-failing due to circuit breaker trip
	ErrCodeRateLimitExceeded   ErrorCode = 6003 // IP or token rate limit hit
	ErrCodePayloadTooLarge     ErrorCode = 6004 // Max payload bytes exceeded
	ErrCodeMalformedPayload    ErrorCode = 6005 // Invalid JSON payload

	// -------------------------------------------------------------
	// 9000s: Internal Server Errors
	// -------------------------------------------------------------
	ErrCodeInternal ErrorCode = 9001 // Unhandled internal server error
)

// Layer returns the architectural layer corresponding to the ErrorCode range.
func (c ErrorCode) Layer() string {
	switch {
	case c >= 1000 && c < 2000:
		return "SecurityLayer"
	case c >= 2000 && c < 3000:
		return "MultiTenancyLayer"
	case c >= 3000 && c < 4000:
		return "FlagEngineLayer"
	case c >= 4000 && c < 5000:
		return "GovernanceLayer"
	case c >= 5000 && c < 6000:
		return "StorageLayer"
	case c >= 6000 && c < 7000:
		return "TransportLayer"
	default:
		return "SystemLayer"
	}
}

// String returns the symbolic name of the ErrorCode.
func (c ErrorCode) String() string {
	switch c {
	case ErrCodeUnauthorized:
		return "UNAUTHORIZED"
	case ErrCodeForbidden:
		return "FORBIDDEN"
	case ErrCodeInvalidCredentials:
		return "INVALID_CREDENTIALS"
	case ErrCodeAPIKeyNotFound:
		return "API_KEY_NOT_FOUND"
	case ErrCodeAPIKeyRevoked:
		return "API_KEY_REVOKED"
	case ErrCodeEnvironmentRestricted:
		return "ENVIRONMENT_RESTRICTED"
	case ErrCodePasswordTooWeak:
		return "PASSWORD_TOO_WEAK"
	case ErrCodeEmailAlreadyExists:
		return "EMAIL_ALREADY_EXISTS"
	case ErrCodeOrgNotFound:
		return "ORGANIZATION_NOT_FOUND"
	case ErrCodeOrgConflict:
		return "ORGANIZATION_CONFLICT"
	case ErrCodeProjectNotFound:
		return "PROJECT_NOT_FOUND"
	case ErrCodeProjectConflict:
		return "PROJECT_CONFLICT"
	case ErrCodeProjectAccessDenied:
		return "PROJECT_ACCESS_DENIED"
	case ErrCodeFlagNotFound:
		return "FLAG_NOT_FOUND"
	case ErrCodeFlagAlreadyExists:
		return "FLAG_ALREADY_EXISTS"
	case ErrCodeInvalidEnvironment:
		return "INVALID_ENVIRONMENT"
	case ErrCodeInvalidRollout:
		return "INVALID_ROLLOUT"
	case ErrCodeEvaluationMissingCtx:
		return "EVALUATION_MISSING_CONTEXT"
	case ErrCodeNotFound:
		return "NOT_FOUND"
	case ErrCodeChangeRequestNotFound:
		return "CHANGE_REQUEST_NOT_FOUND"
	case ErrCodeFourEyesSelfApproval:
		return "FOUR_EYES_SELF_APPROVAL"
	case ErrCodeChangeRequestReviewed:
		return "CHANGE_REQUEST_ALREADY_REVIEWED"
	case ErrCodeExperimentNotFound:
		return "EXPERIMENT_NOT_FOUND"
	case ErrCodeDatabaseConnection:
		return "DATABASE_CONNECTION_ERROR"
	case ErrCodeDatabaseQuery:
		return "DATABASE_QUERY_ERROR"
	case ErrCodeDatabaseConstraint:
		return "DATABASE_CONSTRAINT_VIOLATION"
	case ErrCodeSSEStreamDisconnect:
		return "STREAM_DISCONNECT"
	case ErrCodeCircuitBreakerOpen:
		return "CIRCUIT_BREAKER_OPEN"
	case ErrCodeRateLimitExceeded:
		return "RATE_LIMIT_EXCEEDED"
	case ErrCodePayloadTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case ErrCodeMalformedPayload:
		return "MALFORMED_PAYLOAD"
	default:
		return "INTERNAL_SERVER_ERROR"
	}
}

// AppError is an enterprise-grade error structure implementing the error interface and Unwrap.
type AppError struct {
	Code       ErrorCode `json:"code"`
	Type       string    `json:"type"`
	Layer      string    `json:"layer"`
	Message    string    `json:"message"`
	HTTPStatus int       `json:"status"`
	Err        error     `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s/%d] %s: %v", e.Type, e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s/%d] %s", e.Type, e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError creates a new AppError with code, message, HTTP status, and underlying cause.
func NewAppError(code ErrorCode, message string, httpStatus int, cause error) *AppError {
	return &AppError{
		Code:       code,
		Type:       code.String(),
		Layer:      code.Layer(),
		Message:    message,
		HTTPStatus: httpStatus,
		Err:        cause,
	}
}

// MapSentinelToAppError converts a known domain sentinel error to an AppError.
func MapSentinelToAppError(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	switch {
	case errors.Is(err, ErrUnauthorized):
		return NewAppError(ErrCodeUnauthorized, err.Error(), http.StatusUnauthorized, err)
	case errors.Is(err, ErrForbidden):
		return NewAppError(ErrCodeForbidden, err.Error(), http.StatusForbidden, err)
	case errors.Is(err, ErrEnvironmentRestricted):
		return NewAppError(ErrCodeEnvironmentRestricted, err.Error(), http.StatusForbidden, err)
	case errors.Is(err, ErrEmailAlreadyExists):
		return NewAppError(ErrCodeEmailAlreadyExists, err.Error(), http.StatusConflict, err)
	case errors.Is(err, ErrConflict):
		return NewAppError(ErrCodeFlagAlreadyExists, err.Error(), http.StatusConflict, err)
	case errors.Is(err, ErrUserNotFound):
		return NewAppError(ErrCodeUserNotFound, err.Error(), http.StatusNotFound, err)
	case errors.Is(err, ErrOrgNotFound):
		return NewAppError(ErrCodeOrgNotFound, err.Error(), http.StatusNotFound, err)
	case errors.Is(err, ErrProjectNotFound):
		return NewAppError(ErrCodeProjectNotFound, err.Error(), http.StatusNotFound, err)
	case errors.Is(err, ErrFlagNotFound):
		return NewAppError(ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, err)
	case errors.Is(err, ErrNotFound):
		return NewAppError(ErrCodeNotFound, err.Error(), http.StatusNotFound, err)
	case errors.Is(err, ErrInvalidInput):
		return NewAppError(ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err)
	case errors.Is(err, ErrInvalidEnvironment):
		return NewAppError(ErrCodeInvalidEnvironment, err.Error(), http.StatusBadRequest, err)
	case errors.Is(err, ErrFourEyesSelfApproval):
		return NewAppError(ErrCodeFourEyesSelfApproval, err.Error(), http.StatusBadRequest, err)
	default:
		return NewAppError(ErrCodeInternal, err.Error(), http.StatusInternalServerError, err)
	}
}

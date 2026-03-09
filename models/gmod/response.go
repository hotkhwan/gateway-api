// models/gmod/response.go
package gmod

import (
	"time"

	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/grpmod"
)

// Error codes (normalized, stable contract)
const (
	CodeBadRequest    = "BAD_REQUEST"
	CodeUnauthorized  = "UNAUTHORIZED"
	CodeForbidden     = "FORBIDDEN"
	CodeNotFound      = "NOT_FOUND"
	CodeConflict      = "CONFLICT"
	CodeTooMany       = "TOO_MANY_REQUESTS"
	CodeInternalError = "INTERNAL_ERROR"
	CodeUnavailable   = "SERVICE_UNAVAILABLE"
	CodeTimeout       = "TIMEOUT"
)

// Success codes (optional but recommended)
const (
	CodeSuccess  = "SUCCESS"
	CodeAccepted = "ACCEPTED"
	CodeCreated  = "CREATED"
)

// Response is a generic API response for Swagger documentation
type Response struct {
	Code    string      `json:"code" example:"SUCCESS"`
	Message string      `json:"message" example:"Operation completed"`
	Status  bool        `json:"status" example:"true"`
	Detail  interface{} `json:"detail,omitempty"`
}

type AcceptedResponse struct {
	Status  bool    `json:"status"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Detail  JobInfo `json:"detail"`
}

type OrgListResponse struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Status     bool        `json:"status"`
	Details    interface{} `json:"details"`
	Pagination Pagination  `json:"pagination"`
}

type BadRequestResponse struct {
	Code    string `json:"code" example:"ERROR"`
	Message string `json:"message" example:"Invalid request"`
	Status  bool   `json:"status" example:"false"`
}

type BaseResponse struct {
	Code    string `json:"code"`    // SUCCESS / ERROR CODE
	Message string `json:"message"` // human readable
	Status  bool   `json:"status"`  // true / false
}

type BaseSuccessMessageCreateResponse struct {
	BaseResponse
	ID string `json:"id"`
}

type BaseErrorResponse struct {
	BaseResponse
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
}

type ErrorMessageResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Status  bool   `json:"status"`
}

func ErrorPermifyResponse(msg string, details ...string) ErrorMessageResponse {
	d := ""
	if len(details) > 0 {
		d = details[0]
	}
	return ErrorMessageResponse{
		Code:    "ERROR",
		Message: msg,
		Details: d,
		Status:  false,
	}
}

type BulkError struct {
	Index   int    `json:"index" example:"3"`
	Reason  string `json:"reason" example:"duplicate key"`
	Details string `json:"details,omitempty"`
}

type SuccessMessageBulkCreateResponse struct {
	Code     string      `json:"code" example:"SUCCESS"`
	Message  string      `json:"message" example:"Bulk operation completed"`
	Status   bool        `json:"status" example:"true"`
	Inserted int         `json:"inserted" example:"5"`
	Skipped  int         `json:"skipped" example:"1"`
	IDs      []string    `json:"ids"`
	Errors   []BulkError `json:"errors,omitempty"`
}

// BulkImportError represents a per-row error from XLSX import.
type BulkImportError struct {
	Row       int    `json:"row"`
	RecordKey string `json:"recordKey,omitempty"` // e.g. "AIBOX:channel:30"
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
}

// BulkImportResponse is the response envelope for device management XLSX import.
type BulkImportResponse struct {
	Code     string            `json:"code" example:"SUCCESS"`
	Message  string            `json:"message" example:"Bulk import completed"`
	Status   bool              `json:"status" example:"true"`
	Inserted int               `json:"inserted" example:"5"`
	Updated  int               `json:"updated" example:"2"`
	Skipped  int               `json:"skipped" example:"1"`
	IDs      []string          `json:"ids,omitempty"`
	Errors   []BulkImportError `json:"errors,omitempty"`
}

type InternalErrorResponse struct {
	Message string `json:"message" example:"internal server error"`
}

type DeviceResponse struct {
	Details    []devmod.Device `json:"details"`
	Pagination Pagination      `json:"pagination"`
	Status     bool            `json:"status"`
}

type GroupResponse struct {
	Details    []grpmod.Group `json:"details"`
	Pagination Pagination     `json:"pagination"`
	Status     bool           `json:"status"`
}

// IntrospectDetail ใช้เป็น Detail ใน SuccessDetailResponse
type IntrospectDetail struct {
	Active            bool                      `json:"active"`
	Sub               string                    `json:"sub"`
	Given_name        string                    `json:"given_name,omitempty"`
	Family_name       string                    `json:"family_name,omitempty"`
	PreferredUsername string                    `json:"preferred_username,omitempty"`
	Username          string                    `json:"username"`
	Avatar            string                    `json:"avatar,omitempty"`
	Email             string                    `json:"email,omitempty"`
	Locale            string                    `json:"locale,omitempty"`
	Exp               int64                     `json:"exp"`
	Scope             string                    `json:"scope"`
	Role              string                    `json:"role,omitempty"`
	Permissions       *authmod.PermissionsBlock `json:"permissions,omitempty"`
	MapLocation       *MapLocation              `json:"mapLocation,omitempty"`
	ZoomLevel         int                       `json:"zoomLevel,omitempty"`
}

type MapLocation struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type JobInfo struct {
	JobID     string `json:"jobId"`
	TraceID   string `json:"traceID"`
	MQTTTopic string `json:"mqttTopic"`
}

type Member struct {
	ID        string    `json:"id" bson:"_id,omitempty"`
	Name      string    `json:"name" bson:"name"`
	Email     string    `json:"email,omitempty" bson:"email,omitempty"`
	GroupID   string    `json:"groupId,omitempty" bson:"groupId,omitempty"`
	Role      string    `json:"role,omitempty" bson:"role,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
}

type MembersResponse struct {
	Details    []Member   `json:"details"`
	Pagination Pagination `json:"pagination"`
	Status     bool       `json:"status"`
}

type Pagination struct {
	Page         int    `json:"page"`
	PerPages     int    `json:"perPages"`
	TotalRecords int    `json:"totalRecords"`
	TotalPages   int    `json:"totalPages"`
	SortField    string `json:"sortField"`
	SortOrder    string `json:"sortOrder"`
}

type PaginationDbResponse[T any] struct {
	Details    []T        `json:"details"` // 👈 ใช้ slice ตรงๆ (ไม่เป็น pointer)
	Pagination Pagination `json:"pagination"`
	Status     bool       `json:"status"`
}

// ใช้ struct นี้เฉพาะเพื่อให้ Swag เข้าใจว่า T คือ []Device
type PaginationResponse struct {
	Details    []devmod.Device `json:"details"`
	Pagination Pagination      `json:"pagination"`
	Status     bool            `json:"status" example:"true"`
}

type RolesResponse struct {
	Details    []grpmod.GroupRole `json:"details"`
	Pagination Pagination         `json:"pagination"`
	Status     bool               `json:"status"`
}

type SuccessResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
}

type SuccessDataResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Status  bool        `json:"status"`
	Detail  interface{} `json:"detail,omitempty"`
}

type SuccessDetailResponseWithMSG struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Status  bool        `json:"status"`
	Detail  interface{} `json:"detail,omitempty"`
}

// SuccessDetailsResponse is a response with details (plural) field
type SuccessDetailsResponse struct {
	Code    string      `json:"code" example:"SUCCESS"`
	Message string      `json:"message" example:"resource created successfully"`
	Status  bool        `json:"status" example:"true"`
	Details interface{} `json:"details"`
}

type SuccessDetailResponse[T any] struct {
	Detail T    `json:"detail"`
	Status bool `json:"status"`
}

// SuccessDetailResponseAny is a non-generic version for Swagger documentation
type SuccessDetailResponseAny struct {
	Detail interface{} `json:"detail"`
	Status bool        `json:"status"`
}

type SuccessOptionsDetailResponse[T any] struct {
	Details T    `json:"details"`
	Status  bool `json:"status"`
}

type SuccessMessageResponse struct {
	Code    string `json:"code" example:"SUCCESS"`
	Message string `json:"message" example:"Operation successful"`
	Status  bool   `json:"status" example:"true"`
}

type SuccessMessageCreateResponse struct {
	Code    string `json:"code" example:"SUCCESS"`
	Message string `json:"message" example:"Operation successful"`
	Status  bool   `json:"status" example:"true"`
	ID      string `json:"id" example:"6897202770888774909d2ff5"` // ใช้ string จะสะอาดกว่า
}

type UnauthorizedResponse struct {
	Code    string `json:"code"`
	Message string `json:"message" example:"Invalid or expired token"`
	Status  bool   `json:"status"`
}

type NotFoundResponse struct {
	Code    string `json:"code"`
	Message string `json:"message" example:"Resource not found"`
	Status  bool   `json:"status"`
}

type SuccessDetailResponseIntrospect struct {
	Detail IntrospectDetail `json:"detail"`
	Status bool             `json:"status"`
}

// SuccessDetailResponseMenuAccess is the response for menu access endpoints
type SuccessDetailResponseMenuAccess struct {
	Code    string      `json:"code" example:"SUCCESS"`
	Message string      `json:"message" example:"accessible menus fetched"`
	Status  bool        `json:"status" example:"true"`
	Detail  interface{} `json:"detail"`
}

// SuccessDetailResponseResourceAccess is the response for resource access endpoints
type SuccessDetailResponseResourceAccess struct {
	Code    string      `json:"code" example:"SUCCESS"`
	Message string      `json:"message" example:"accessible cameras fetched"`
	Status  bool        `json:"status" example:"true"`
	Detail  interface{} `json:"detail"`
}

// PaginatedResponse is a response with pagination metadata
type PaginatedResponse struct {
	Code       string      `json:"code" example:"SUCCESS"`
	Message    string      `json:"message" example:"profiles fetched"`
	Status     bool        `json:"status" example:"true"`
	Details    interface{} `json:"details"`
	Pagination Pagination  `json:"pagination"`
}

type ApiErrorResponse struct {
	Code    string      `json:"code" example:"BAD_REQUEST"`
	Message string      `json:"message" example:"Invalid request"`
	Details interface{} `json:"details,omitempty"`
	Status  bool        `json:"status" example:"false"`
}

// Organization
type InviteUserRequest struct {
	UserId string `json:"userId"`
	Role   string `json:"role"` // "member" | "admin"
}

type InviteUserResponse struct {
	Code    string `json:"code"`
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Details struct {
		Id      string `json:"orgId"`
		UserId  string `json:"userId"`
		Role    string `json:"role"`
		Invited bool   `json:"invited"`
	} `json:"details"`
}

type OrgMember struct {
	UserId string `json:"userId"`
	Role   string `json:"role"`
}

// PromoteToOwnerResponse is the response for promoting a user to owner
type PromoteToOwnerResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Details struct {
		OrgId      string `json:"orgId"`
		UserId     string `json:"userId"`
		Role       string `json:"role"` // "owner"
		PromotedAt int64  `json:"promotedAt"`
	} `json:"details"`
}

// DemoteFromOwnerRequest is the request for demoting an owner
type DemoteFromOwnerRequest struct {
	NewRole string `json:"newRole"` // "member" or "admin"
}

// DemoteFromOwnerResponse is the response for demoting an owner
type DemoteFromOwnerResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Details struct {
		OrgId        string `json:"orgId"`
		UserId       string `json:"userId"`
		PreviousRole string `json:"previousRole"` // "owner"
		NewRole      string `json:"newRole"`      // "member" or "admin"
		DemotedAt    int64  `json:"demotedAt"`
	} `json:"details"`
}

// TransferBillingOwnershipRequest is the request for transferring billing ownership
type TransferBillingOwnershipRequest struct {
	NewBillingOwnerId string `json:"newBillingOwnerId"`
}

// TransferBillingOwnershipResponse is the response for transferring billing ownership
type TransferBillingOwnershipResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Details struct {
		OrgId           string `json:"orgId"`
		PreviousOwnerId string `json:"previousOwnerId"`
		NewOwnerId      string `json:"newOwnerId"`
		TransferredAt   int64  `json:"transferredAt"`
	} `json:"details"`
}

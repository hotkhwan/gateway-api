// models/user.go
package usrmod

import (
	"klynx/models/gmod"
	"time"
)

type User struct {
	ID          string       `json:"id" bson:"_id"`
	Username    string       `json:"username" bson:"username"`
	Email       string       `json:"email" bson:"email"`
	FirstName   string       `json:"firstName" bson:"firstName"`
	LastName    string       `json:"lastName" bson:"lastName"`
	Roles       string       `json:"roles" bson:"roles"`
	Avatar      string       `json:"avatar" bson:"avatar"`
	Locale      string       `json:"locale" bson:"locale"`
	Plant       string       `json:"plant" bson:"plant"`
	PerPages    int          `json:"perPages" bson:"perPages"`
	RefId       string       `json:"refId" bson:"refId"`
	Department  string       `json:"department" bson:"department"`
	Enabled     bool         `json:"enabled" bson:"enabled"`
	MapLocation *MapLocation `json:"mapLocation,omitempty"`
	CreatedAt   *time.Time   `json:"createdAt,omitempty"`
}

type UserAttributes struct {
	Avatar     []string `json:"avatar,omitempty"`
	Locale     []string `json:"locale,omitempty"`
	Permission []string `json:"permission,omitempty"`
	Role       []string `json:"role,omitempty"`
	Plant      []string `json:"plant,omitempty"`
	PerPage    []string `json:"perpage,omitempty"` // อาจต้องดึงและแปลงทีหลัง
}

type UserAccess struct {
	Impersonate           bool `json:"impersonate"`
	Manage                bool `json:"manage"`
	ManageGroupMembership bool `json:"manageGroupMembership"`
	MapRoles              bool `json:"mapRoles"`
	View                  bool `json:"view"`
}

// ✅ ใช้ gmod.Pagination แทนของตัวเอง
type UserListResult struct {
	Details    []User          `json:"details"`
	Pagination gmod.Pagination `json:"pagination"`
	Status     bool            `json:"status"`
}

type ChangePasswordRequest struct {
	Password string `json:"password" example:"P@ssw0rd123"`
}

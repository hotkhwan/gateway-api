package memmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Member struct {
	ID        string `json:"id" bson:"_id,omitempty"`
	GroupID   string `json:"groupID" bson:"groupID"`
	Group     string `json:"group" bson:"group"`
	UserID    string `json:"userID" bson:"userID"`
	Role      string `json:"role" bson:"role"`
	CreatedAt string `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
}
type MemberMongo struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	GroupID   string             `json:"groupID" bson:"groupID"`
	Group     string             `json:"group" bson:"group"`
	UserID    string             `json:"userID" bson:"userID"`
	Role      string             `json:"role" bson:"role"`
	CreatedAt time.Time          `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
}
type MemberRequest struct {
	GroupID string `json:"groupID" bson:"groupID"`
	Group   string `json:"group" bson:"group"`
	UserID  string `json:"userID" bson:"userID"`
	Role    string `json:"role" bson:"role"`
}
type MemberRequest2 struct {
	GroupID string   `json:"groupID"`
	UserIds []string `json:"userIds"`
}
type Pagination struct {
	Page         int    `json:"page"`
	PerPage      int    `json:"perPage"`
	TotalRecords int    `json:"totalRecords"`
	TotalPages   int    `json:"totalPages"`
	SortField    string `json:"sortField"`
	SortOrder    string `json:"sortOrder"`
}

type MemberListResponse struct {
	Details    []Member   `json:"details"`
	Pagination Pagination `json:"pagination"`
	Status     bool       `json:"status"`
}

type MemberResults struct {
	Details []MemberResult `json:"details"`
	Status  bool           `json:"status"`
}
type MemberResult struct {
	GroupID string   `json:"groupID" bson:"groupID"`
	Group   string   `json:"group" bson:"group"`
	Member  []Member `json:"member" bson:"member"`
}
type MemberDetail struct {
	UserID    string `json:"userID" bson:"userID"`
	Role      string `json:"role" bson:"role"`
	FirstName string `json:"firstName" bson:"firstName"`
	LastName  string `json:"lastName" bson:"lastName"`
	Enabled   bool   `json:"enabled" bson:"enabled"`
}

type MemberDetailResponse struct {
	GroupID string         `json:"groupID"`
	Users   []MemberDetail `json:"users"`
	Status  bool           `json:"status"`
}

type GroupDetails struct {
	GroupID string   `json:"groupID"`
	UserIDs []string `json:"userIds"`
}

type UserDetail struct {
	UserID    string `json:"userID"`
	Role      string `json:"role"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

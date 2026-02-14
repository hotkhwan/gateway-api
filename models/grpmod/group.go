// models/grpmod/group.go
package grpmod

import "time"

type Group struct {
	ID          string        `json:"id" bson:"_id,omitempty"`
	Name        string        `json:"name" bson:"name"`
	Description string        `json:"description,omitempty" bson:"description,omitempty"`
	Icon        string        `json:"icon,omitempty" bson:"icon,omitempty"`
	Members     []GroupMember `json:"members,omitempty" bson:"members,omitempty"`
	ParentID    *string       `json:"parentId,omitempty" bson:"parentId,omitempty"`
	Public      bool          `json:"public" bson:"public"`
	CreatedAt   time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
}
type GroupRequest struct {
	Name        string  `json:"name"`
	ParentID    *string `json:"parentId,omitempty"` // optional
	Public      *bool   `json:"public"`             // optional
	Icon        string  `json:"icon,omitempty"`     // optional
	Description string  `json:"description,omitempty"`
}

type GroupMember struct {
	UserID   string `json:"userId" bson:"userId"`
	Username string `json:"username" bson:"username"`
	FullName string `json:"fullName" bson:"fullName"`
}

type GroupTree struct {
	ID          string        `json:"id" example:"685e34ee49e00a250204b8eb"`
	Name        string        `json:"name" example:"analytic"`
	Description string        `json:"description,omitempty" example:"กลุ่มกล้องวิเคราะห์"`
	Icon        string        `json:"icon,omitempty" bson:"icon,omitempty"`
	Public      bool          `json:"public"`
	Members     []GroupMember `json:"members,omitempty"`
	Children    []GroupTree   `json:"children,omitempty"`
}

type GroupTreeResponse struct {
	Details []GroupTree `json:"details"` // เปลี่ยนตรงนี้
	Status  bool        `json:"status" example:"true"`
}

type GroupRole struct {
	ID          string    `json:"id" bson:"_id,omitempty"`
	Role        string    `json:"role" bson:"role"`
	GroupID     string    `json:"groupId,omitempty" bson:"groupId,omitempty"`
	Description string    `json:"description,omitempty" bson:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
}

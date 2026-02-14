// models/authzmod/grant.go
package authzmod

// ✅ GrantGroupMenu: สำหรับ grant menu ให้ group (เช่น menu_admin_1 → group A)
type GrantGroupMenu struct {
	GroupID  string `json:"groupId"`
	MenuType string `json:"menuType"` // เช่น "menu_admin"
	MenuID   string `json:"menuId"`   // เช่น "menu_admin_1" หรือ "menu_admin_global"
}

// ✅ GrantGroupUser: สำหรับ grant user เข้ากลุ่ม (เช่น user U1 → group G1)
type GrantGroupUser struct {
	UserID  string `json:"userId"`
	GroupID string `json:"groupId"`
}

// ✅ UpdateGroupMenu: สำหรับเปลี่ยนเมนูของ group (จาก old → new menu)
type UpdateGroupMenu struct {
	GroupID   string `json:"groupId"`
	MenuType  string `json:"menuType"`
	OldMenuID string `json:"oldMenuId"`
	NewMenuID string `json:"newMenuId"`
}

// ✅ GrantResourceRequest: ใช้ grant สิทธิ์บน resource (device, iot, menu) ให้กับ user/group/role
type GrantResourceRequest struct {
	ResourceID string `json:"resourceId"`

	// 💡 Relations
	OwnerUserID     string   `json:"ownerUserId,omitempty"`     // → relation: owner
	EditorUserIDs   []string `json:"editorUserIds,omitempty"`   // → relation: editor
	ViewerUserIDs   []string `json:"viewerUserIds,omitempty"`   // → relation: viewer
	AssignedGroupID string   `json:"assignedGroupId,omitempty"` // → relation: assigned_group
	AssignedRoleID  string   `json:"assignedRoleId,omitempty"`  // → relation: assigned_role

	// 💡 Attributes
	Type string `json:"type,omitempty"` // เช่น "device", "menu", ใช้ใน attribute:type
	Crud string `json:"crud,omitempty"` // เช่น "c", "cru", "crd", ใช้ใน attribute:crud

	// ✅ เพิ่ม field นี้เข้าไปเพื่อรองรับ custom tuple เช่น group.member, sensor.located_in_group
	ExtraTuples []map[string]interface{} `json:"-"`
}

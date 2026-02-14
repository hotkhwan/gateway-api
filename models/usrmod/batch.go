// models/usrmod/batch.go
package usrmod

type BatchGetUsersRequest struct {
	UserIds []string `json:"userIds"`
}

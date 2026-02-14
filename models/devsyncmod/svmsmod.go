// models/devsyncmod/svmsmod.go
package devsyncmod

type SVMSSyncResponse struct {
	Code    string     `json:"code"`
	Message string     `json:"message"`
	Status  bool       `json:"status"`
	Detail  SyncResult `json:"detail"`
}

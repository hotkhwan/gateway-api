// models/devsyncmod/ibocmod.go
package devsyncmod

type IBOCSyncResponse struct {
	Code    string     `json:"code"`
	Message string     `json:"message"`
	Status  bool       `json:"status"`
	Detail  SyncResult `json:"detail"`
}

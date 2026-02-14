// internal/gateways/watchman/watchgw/types.go
package watchgw

import "encoding/json"

type Resp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
}

type watchmanResp struct {
	Status  bool   `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details *struct {
		ID json.Number `json:"id"`
	} `json:"details,omitempty"`
}

type getByIDCardResp struct {
	Status  bool   `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details *struct {
		ID json.Number `json:"id"`
	} `json:"details,omitempty"`
}

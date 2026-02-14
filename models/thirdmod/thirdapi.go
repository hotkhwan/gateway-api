// models/thirdmod/third.go
package thirdmod

// ---------- Create Service Account Client ----------

type CreateServiceAccountReq struct {
	Description string `json:"description,omitempty"`
}

type CreateServiceAccountInput struct {
	ClientID    string
	Description string
}

type CreateServiceAccountOutput struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// ✅ ทำ Res เป็น alias ของ Output เพื่อลดการ copy field และแก้ S1016
type CreateServiceAccountRes = CreateServiceAccountOutput

// ---------- Client Credentials Token ----------

type ClientCredentialsReq struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Scope        string `json:"scope,omitempty"` // optional
}

type ClientCredentialsToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope,omitempty"`
}

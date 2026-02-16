// internal/gateways/authzgw/types.go
package authzgw

type EntityRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type SubjectRef struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Relation string `json:"relation,omitempty"`
}

type Relationship struct {
	Entity   EntityRef  `json:"entity"`
	Relation string     `json:"relation"`
	Subject  SubjectRef `json:"subject"`
}

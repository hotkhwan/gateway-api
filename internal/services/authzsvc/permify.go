// internal/services/authzsvc/permify.go
package authzsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"klynx/config"
	"klynx/models/authzmod"
)

// ---- Permify REST client ----

type PermifyClient struct {
	baseURL string
	tenant  string
	httpc   *http.Client
}

// ใช้ค่าจาก config.InitPermifyRest() ที่เซ็ตไว้แล้ว (ไม่พึ่ง ENV)
func NewPermifyClient() *PermifyClient {
	base := strings.TrimRight(config.PermifyBaseURL, "/")
	tenant := config.PermifyTenantID
	if base == "" || tenant == "" {
		return nil
	}
	return &PermifyClient{
		baseURL: base,
		tenant:  tenant,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

// ---- Tuple types ----

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

// ใช้ map[string]interface{} เพื่อส่งต่อไป /data/write, /data/delete ได้เลย
type TupleChange struct {
	Action string                 `json:"action"` // create | delete
	Tuple  map[string]interface{} `json:"tuple"`  // {entity:{type,id}, relation:"...", subject:{type,id}}
}

type TuplePlan struct {
	ProfileCode string         `json:"profileCode"`
	Version     int            `json:"version"`
	Changes     []TupleChange  `json:"changes"`
	Stats       map[string]int `json:"stats"`
}

// ---- Desired set & diff ----

type desiredTupleSet map[string]Relationship // key = stableKey

func (d desiredTupleSet) Scope() []EntityRef {
	m := map[string]EntityRef{}
	for _, r := range d {
		m[r.Entity.Type+":"+r.Entity.ID] = r.Entity
	}
	out := make([]EntityRef, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	return out
}

func stableKey(r Relationship) string {
	return r.Entity.Type + ":" + r.Entity.ID + "#" + r.Relation + "@" + r.Subject.Type + ":" + r.Subject.ID
}

// แปลง Relationship → map[string]interface{} ให้ตรง payload REST
func relToMap(r Relationship) map[string]interface{} {
	return map[string]interface{}{
		"entity": map[string]interface{}{
			"type": r.Entity.Type,
			"id":   r.Entity.ID,
		},
		"relation": r.Relation,
		"subject": map[string]interface{}{
			"type": r.Subject.Type,
			"id":   r.Subject.ID,
		},
	}
}

// ใช้กับ authzmod.ProfileItem โดยตรง (มี field Resource, Action, Subject)
// 🆕 ปรับให้ตรง schema ล่าสุด:
//
//	entity resource {
//	  relation viewer @user @role @group
//	  relation editor @user @role @group
//	  relation owner  @user @role @group
//	  relation assigned_group @group
//	}
func BuildDesiredTuplesFromItems(items []authzmod.ProfileItem) desiredTupleSet {
	out := desiredTupleSet{}
	for _, it := range items {
		resID := strings.TrimSpace(it.Resource)
		if resID == "" {
			continue
		}
		action := strings.ToLower(strings.TrimSpace(it.Action))
		subject := strings.TrimSpace(it.Subject) // "role:PF_OFFICE" | "group:g_inno"

		var relation string
		switch action {
		case "read", "view":
			// ผู้มีสิทธิ์อ่าน → relation viewer
			relation = "viewer"
		case "update", "edit":
			// ผู้มีสิทธิ์แก้ไข → relation editor
			relation = "editor"
		case "delete", "remove":
			// เจ้าของ → relation owner
			relation = "owner"
		case "public":
			// public → assigned_group กับ group:g_public
			relation = "assigned_group"
			subject = "group:g_public"
		default:
			// action แปลก ๆ ไม่รู้ map ยังไง → ข้าม
			continue
		}

		subType, subID := parseSubject(subject)
		if subType == "" || subID == "" {
			continue
		}
		t := Relationship{
			Entity:   EntityRef{Type: "resource", ID: resID},
			Relation: relation,
			Subject:  SubjectRef{Type: subType, ID: subID},
		}
		out[stableKey(t)] = t
	}
	return out
}

func parseSubject(s string) (typ, id string) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

// 🆕 ปรับ filter delete ให้ตรง relation จริงใน schema
func DiffTuples(profileCode string, version int, desired desiredTupleSet, existing []Relationship) *TuplePlan {
	ex := map[string]Relationship{}
	for _, r := range existing {
		ex[stableKey(r)] = r
	}

	var creates []TupleChange
	var deletes []TupleChange

	// create = desired แต่ยังไม่มีใน existing
	for k, r := range desired {
		if _, ok := ex[k]; !ok {
			creates = append(creates, TupleChange{
				Action: "create",
				Tuple:  relToMap(r),
			})
		}
	}

	// delete = existing ที่ไม่มีใน desired และเป็น relation ที่เราดูแลจริง ๆ
	for _, r := range existing {
		k := stableKey(r)
		if _, ok := desired[k]; !ok {
			if r.Entity.Type == "resource" &&
				(r.Relation == "viewer" ||
					r.Relation == "editor" ||
					r.Relation == "owner" ||
					r.Relation == "assigned_group") {

				deletes = append(deletes, TupleChange{
					Action: "delete",
					Tuple:  relToMap(r),
				})
			}
		}
	}

	stats := map[string]int{
		"create": len(creates),
		"delete": len(deletes),
	}
	changes := append(creates, deletes...)

	return &TuplePlan{
		ProfileCode: profileCode,
		Version:     version,
		Changes:     changes,
		Stats:       stats,
	}
}

// ---- Permify REST calls ----

func (c *PermifyClient) ReadExistingTuples(ctx context.Context, scope []EntityRef) ([]Relationship, error) {
	if c == nil {
		return nil, fmt.Errorf("permify client is nil")
	}

	// ✅ ถ้าไม่มี resource ใด ๆ ให้เช็คอยู่แล้ว → ถือว่าไม่มี existing tuples, ไม่ต้องยิง Permify
	if len(scope) == 0 {
		return []Relationship{}, nil
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/read", c.baseURL, c.tenant)

	// ---- ตาม spec Permify v1.1 ----
	type entityFilter struct {
		Type string   `json:"type"`
		IDs  []string `json:"ids"`
	}
	type tupleFilter struct {
		Entity   *entityFilter `json:"entity,omitempty"`
		Relation string        `json:"relation,omitempty"`
		Subject  interface{}   `json:"subject,omitempty"`
	}
	type req struct {
		Metadata map[string]string `json:"metadata,omitempty"`
		Filter   *tupleFilter      `json:"filter,omitempty"`
		// page_size / continuous_token ยังไม่ใช้ตอนนี้
	}

	schemaVersion := config.CurrentSchemaVersion
	if schemaVersion == "" {
		schemaVersion = "latest"
	}

	// ✅ ตอนนี้รองรับครั้งละ type เดียว (ส่วนใหญ่คือ "resource")
	firstType := scope[0].Type
	ids := make([]string, 0, len(scope))
	for _, e := range scope {
		if e.Type != firstType {
			continue
		}
		ids = append(ids, e.ID)
	}

	if len(ids) == 0 {
		return []Relationship{}, nil
	}

	r := req{
		Metadata: map[string]string{
			"schema_version": schemaVersion,
		},
		Filter: &tupleFilter{
			Entity: &entityFilter{
				Type: firstType,
				IDs:  ids,
			},
		},
	}

	body, _ := json.Marshal(r)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("permify read failed: %s %s", resp.Status, string(b))
	}

	var out struct {
		Tuples []Relationship `json:"tuples"` // ✅ field ชื่อ tuples ตาม docs
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Tuples, nil
}

// ---- Low-level REST helpers (write / delete) ----

// เขียน relationships / attributes
func writeDataREST(ctx context.Context, tuplesUpserts []map[string]interface{}, attrUpserts []map[string]interface{}) error {
	if len(tuplesUpserts) == 0 && len(attrUpserts) == 0 {
		return nil
	}

	// ✅ สร้าง metadata เสมอ (ไม่ให้เป็น nil)
	meta := map[string]interface{}{}
	if config.CurrentSchemaVersion != "" {
		// ถ้าอนาคตมีการเซ็ต version จริง ๆ จาก ApplySchema ก็จะไปตรงนี้
		meta["schema_version"] = config.CurrentSchemaVersion
	}

	payload := map[string]interface{}{
		// ❗ ใส่ metadata แม้จะว่าง เพื่อผ่าน validation "value is required"
		"metadata": meta,
	}

	if len(tuplesUpserts) > 0 {
		payload["tuples"] = tuplesUpserts
	}
	if len(attrUpserts) > 0 {
		payload["attributes"] = attrUpserts
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/v1/tenants/%s/data/write", config.PermifyBaseURL, config.PermifyTenantID)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("write failed: status=%d resp=%s", res.StatusCode, string(body))
	}
	return nil
}

// ลบ tuple แบบระบุ entity+relation+subject ให้ตรงกับ Permify REST v1.1
func deleteTupleREST(ctx context.Context, tuple map[string]interface{}) error {
	entityRaw, _ := tuple["entity"].(map[string]interface{})
	subjectRaw, _ := tuple["subject"].(map[string]interface{})
	relation, _ := tuple["relation"].(string)

	entType, _ := entityRaw["type"].(string)
	entID, _ := entityRaw["id"].(string)
	subType, _ := subjectRaw["type"].(string)
	subID, _ := subjectRaw["id"].(string)

	if entType == "" || entID == "" || subType == "" || subID == "" || relation == "" {
		return fmt.Errorf("deleteTupleREST: invalid tuple payload: %+v", tuple)
	}

	schemaVersion := config.CurrentSchemaVersion
	if schemaVersion == "" {
		schemaVersion = "latest"
	}

	payload := map[string]interface{}{
		"metadata": map[string]interface{}{
			"schema_version": schemaVersion,
		},
		"tuple_filter": map[string]interface{}{
			"entity": map[string]interface{}{
				"type": entType,
				"ids":  []string{entID},
			},
			"relation": relation,
			"subject": map[string]interface{}{
				"type": subType,
				"ids":  []string{subID},
			},
		},
		// 👇 สำคัญ: เติม attribute_filter ให้มี value
		"attribute_filter": map[string]interface{}{
			"entity": map[string]interface{}{
				"type": entType,
				"ids":  []string{entID},
			},
		},
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/v1/tenants/%s/data/delete",
		strings.TrimRight(config.PermifyBaseURL, "/"),
		config.PermifyTenantID,
	)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("delete failed: status=%d resp=%s", res.StatusCode, string(body))
	}

	return nil
}

// อ่าน tuples ทั้ง tenant ทีละ page (ไม่ใส่ filter เฉพาะ entity/subject)
// ใช้สำหรับ factory reset / scan ทั้ง tenant
func (c *PermifyClient) ReadAllTuplesOnce(ctx context.Context, pageSize int32, continuousToken string) ([]Relationship, string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("permify client is nil")
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/read", c.baseURL, c.tenant)

	// ❗ Permify บังคับว่าต้องมี Filter (แม้จะว่างก็ตาม)
	type req struct {
		Metadata        map[string]interface{} `json:"metadata,omitempty"`
		Filter          map[string]interface{} `json:"filter"` // <<== ไม่ใช่ omitempty แล้ว
		PageSize        int32                  `json:"page_size,omitempty"`
		ContinuousToken string                 `json:"continuous_token,omitempty"`
	}

	schemaVersion := config.CurrentSchemaVersion
	if schemaVersion == "" {
		schemaVersion = "latest"
	}

	r := req{
		Metadata: map[string]interface{}{
			"schema_version": schemaVersion,
		},
		// ⚠️ ตรงนี้คือ key เลย — ส่ง filter เป็น object ว่าง ๆ
		Filter:   map[string]interface{}{},
		PageSize: pageSize,
	}
	if continuousToken != "" {
		r.ContinuousToken = continuousToken
	}

	body, _ := json.Marshal(r)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpc.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("permify read-all failed: %s %s", resp.Status, string(b))
	}

	var out struct {
		Tuples          []Relationship `json:"tuples"`
		ContinuousToken string         `json:"continuous_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}

	return out.Tuples, out.ContinuousToken, nil
}

// func (s *ProfileService) DeleteProfilePermissions(ctx context.Context, profileCode string) error {
// 	// 1. ดึงสิทธิ์ทั้งหมดที่มี profileCode นี้อยู่
// 	tuples, err := s.effTupleRepo.GetTuplesByProfile(ctx, profileCode)
// 	if err != nil {
// 		return err
// 	}

// 	for _, t := range tuples {
// 		// 2. ตรวจสอบว่า profileCode นี้เป็น "อันสุดท้าย" หรือไม่
// 		// ถ้านับได้ 1 และตัวในนั้นคือ profileCode ที่เรากำลังจะลบ
// 		isLastOwner := len(t.ProfileCodes) == 1 && t.ProfileCodes[0] == profileCode

// 		if isLastOwner {
// 			// ✅ เป็นคนสุดท้าย -> ยิงลบที่ Permify จริงๆ
// 			err := s.DeleteTupleREST(
// 				ctx,
// 				t.EntityType,
// 				t.EntityID,
// 				t.Relation,
// 				t.SubjectType,
// 				t.SubjectID,
// 			)

// 			if err != nil {
// 				log.Printf("❌ Permify Error: %v", err)
// 				continue // เก็บไว้ใน Mongo ก่อน เพื่อ Retry รอบหน้า
// 			}

// 			// เมื่อ Permify ลบสำเร็จ -> ลบข้อมูลออกจาก Mongo ไปเลย
// 			s.effTupleRepo.DeleteEffectiveTuple(ctx, t.ID)

// 		} else {
// 			// ✅ มีโปรไฟล์อื่นใช้อยู่ด้วย (เช่นมี pf_cam1, pf_admin)
// 			// แค่ถอนชื่อ pf_cam1 ออกจากอาเรย์ใน Mongo เท่านั้น
// 			err := s.effTupleRepo.PullProfileCode(ctx, t.ID, profileCode)
// 			if err != nil {
// 				log.Printf("❌ Mongo Pull Error: %v", err)
// 			}
// 		}
// 	}
// 	return nil
// }

// internal/gateways/authgw/roles.go
package authgw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

type RealmRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func AssignRealmRole(ctx context.Context, token, userID, roleName string) error {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	// 1) get role
	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimRight(kcURL, "/")+
			"/admin/realms/"+realm+
			"/roles/"+roleName,
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("realm role not found")
	}

	var role RealmRole
	json.NewDecoder(resp.Body).Decode(&role)

	// 2) map role
	b, _ := json.Marshal([]RealmRole{role})
	req2, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(kcURL, "/")+
			"/admin/realms/"+realm+
			"/users/"+userID+
			"/role-mappings/realm",
		bytes.NewReader(b),
	)
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := client.Do(req2)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusNoContent {
		return errors.New("assign role failed")
	}
	return nil
}

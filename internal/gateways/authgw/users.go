// internal/gateways/authgw/users.go
package authgw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type KCUser struct {
	ID string `json:"id"`
}

func CreateUser(ctx context.Context, token string, payload any) error {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	b, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(kcURL, "/")+
			"/admin/realms/"+realm+"/users",
		bytes.NewReader(b),
	)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create user failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func FindUserIDByUsername(ctx context.Context, token, username string) (string, error) {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	u := strings.TrimRight(kcURL, "/") +
		"/admin/realms/" + realm +
		"/users"

	q := url.Values{}
	q.Set("username", username)
	q.Set("exact", "true")

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		u+"?"+q.Encode(),
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var users []KCUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return "", err
	}
	if len(users) == 0 {
		return "", errors.New("user not found after create")
	}
	return users[0].ID, nil
}

func SetPassword(ctx context.Context, token, userID, password string) error {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	payload := map[string]any{
		"type":      "password",
		"value":     password,
		"temporary": false,
	}

	b, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		strings.TrimRight(kcURL, "/")+
			"/admin/realms/"+realm+
			"/users/"+userID+"/reset-password",
		bytes.NewReader(b),
	)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return errors.New("reset password failed")
	}
	return nil
}

func ChangePassword(
	ctx context.Context,
	token string,
	userID string,
	password string,
	temporary bool,
) error {

	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	payload := map[string]any{
		"type":      "password",
		"value":     password,
		"temporary": temporary,
	}

	b, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fmt.Sprintf(
			"%s/admin/realms/%s/users/%s/reset-password",
			strings.TrimRight(kcURL, "/"),
			realm,
			userID,
		),
		bytes.NewReader(b),
	)

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"change password failed (%d): %s",
			resp.StatusCode,
			string(body),
		)
	}

	return nil
}

func UpdateUserAttributes(ctx context.Context, token, userID string, attrs map[string]any) error {
	payload := map[string]any{"attributes": attrs}
	return putUser(ctx, token, userID, payload)
}

func SetEnabled(ctx context.Context, token, userID string, enabled bool) error {
	payload := map[string]any{"enabled": enabled}
	return putUser(ctx, token, userID, payload)
}

func putUser(ctx context.Context, token, userID string, payload map[string]any) error {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fmt.Sprintf("%s/admin/realms/%s/users/%s",
			strings.TrimRight(kcURL, "/"), realm, userID),
		bytes.NewReader(b),
	)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("keycloak update failed: %s", body)
	}
	return nil
}

func DeleteUser(ctx context.Context, token, userID string) error {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		fmt.Sprintf("%s/admin/realms/%s/users/%s",
			strings.TrimRight(kcURL, "/"), realm, userID),
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete user failed (%d)", resp.StatusCode)
	}
	return nil
}

func GetUser(
	ctx context.Context,
	token string,
	userID string,
) (map[string]any, error) {

	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/admin/realms/%s/users/%s",
			strings.TrimRight(kcURL, "/"), realm, userID),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user failed (%d): %s", resp.StatusCode, string(body))
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func PutUser(
	ctx context.Context,
	token string,
	userID string,
	payload map[string]any,
) error {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fmt.Sprintf("%s/admin/realms/%s/users/%s",
			strings.TrimRight(kcURL, "/"), realm, userID),
		bytes.NewReader(b),
	)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("keycloak update failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

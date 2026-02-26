// internal/gateways/authgw/id.go
package authgw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Config struct {
	BaseURL      string
	Realm        string
	ClientID     string
	ClientSecret string
}

type Client struct {
	cfg      Config
	httpc    *http.Client
	token    string
	expireAt time.Time
	mu       sync.Mutex
}

type UserProfile struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Enabled   bool   `json:"enabled"`
}

func New(cfg Config) *Client {
	return &Client{
		cfg:   cfg,
		httpc: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) getAdminToken(ctx context.Context) (string, error) {

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expireAt) {
		return c.token, nil
	}

	form := "grant_type=client_credentials" +
		"&client_id=" + c.cfg.ClientID +
		"&client_secret=" + c.cfg.ClientSecret

	url := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		strings.TrimRight(c.cfg.BaseURL, "/"),
		c.cfg.Realm,
	)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	c.token = tr.AccessToken
	c.expireAt = time.Now().Add(time.Duration(tr.ExpiresIn-30) * time.Second)

	return c.token, nil
}

func (c *Client) GetUsersByIds(
	ctx context.Context,
	userIds []string,
) ([]UserProfile, error) {

	token, err := c.getAdminToken(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]UserProfile, 0, len(userIds))

	for _, id := range userIds {

		url := fmt.Sprintf("%s/admin/realms/%s/users/%s",
			strings.TrimRight(c.cfg.BaseURL, "/"),
			c.cfg.Realm,
			id,
		)

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpc.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var u UserProfile
		if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
			continue
		}

		out = append(out, u)
	}

	return out, nil
}

// SetUserActiveOrg sets the active organization ID for a user via Keycloak user attributes
func (c *Client) SetUserActiveOrg(
	ctx context.Context,
	tenantId string,
	userId string,
	orgId string,
) error {

	token, err := c.getAdminToken(ctx)
	if err != nil {
		return err
	}

	// Get the current user to preserve existing attributes
	getURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		strings.TrimRight(c.cfg.BaseURL, "/"),
		c.cfg.Realm,
		userId,
	)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("failed to get user: status %d", resp.StatusCode)
	}

	var user map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		resp.Body.Close()
		return err
	}
	resp.Body.Close()

	// Get existing attributes
	existingAttrs := map[string]any{}
	if raw, ok := user["attributes"].(map[string]any); ok {
		existingAttrs = raw
	}

	// Update activeOrgId attribute
	existingAttrs["activeOrgId"] = []string{orgId}

	// Put the updated user
	payload := map[string]any{
		"attributes": existingAttrs,
	}

	putURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		strings.TrimRight(c.cfg.BaseURL, "/"),
		c.cfg.Realm,
		userId,
	)

	body, _ := json.Marshal(payload)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPut, putURL, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to update user: status %d", resp.StatusCode)
	}

	return nil
}

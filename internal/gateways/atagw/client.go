// internal/gateways/atagw/client.go
package atagw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hotkhwan/gateway-api/internal/gateways/gwcom"
	"github.com/hotkhwan/gateway-api/models/aimodel"
)

type Client struct {
	BaseURL string
}

func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	payload := map[string]string{
		"name":     username,
		"password": password,
	}
	b, _ := json.Marshal(payload)

	body, status, err := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/user/login",
		nil,
		bytes.NewReader(b),
	)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("login failed status=%d", status)
	}

	var res struct {
		ErrCode int `json:"errCode"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", err
	}
	if res.ErrCode != 0 {
		return "", fmt.Errorf("login errCode=%d", res.ErrCode)
	}
	return res.Data.Token, nil
}

func (c *Client) GetChannels(
	ctx context.Context,
	token string,
	deviceID int,
	page, size int,
) ([]aimodel.ATAChannel, error) {

	req := map[string]int{
		"pageIndex": page,
		"pageSize":  size,
		"deviceId":  deviceID,
	}
	b, _ := json.Marshal(req)

	body, status, err := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/channel/get-channels",
		map[string]string{
			"Token": token,
		},
		bytes.NewReader(b),
	)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("get-channels status=%d", status)
	}

	var res struct {
		ErrCode int `json:"errCode"`
		Data    struct {
			List []aimodel.ATAChannel `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	if res.ErrCode != 0 {
		return nil, fmt.Errorf("get-channels errCode=%d", res.ErrCode)
	}
	return res.Data.List, nil
}

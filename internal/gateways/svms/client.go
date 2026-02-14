// internal/gateways/svms/client.go
package svms

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	User    string
	Pass    string
	Token   string
	http    *http.Client
}

func New(baseURL, user, pass string) *Client {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Client{
		BaseURL: baseURL,
		User:    user,
		Pass:    pass,
		http:    &http.Client{Timeout: 10 * time.Second, Transport: tr},
	}
}

type loginResp struct {
	Result     int    `json:"result"`
	Token      string `json:"token"`
	ErrorCount int    `json:"errorcount"`
}

func (c *Client) loginOnce(ctx context.Context, path string) (*loginResp, *http.Response, error) {
	// 1) base64 password
	b64 := base64.StdEncoding.EncodeToString([]byte(c.Pass))

	// 2) ใช้ url.Values เพื่อให้ URL-encode อักขระใน base64 เช่น + / =
	q := url.Values{}
	q.Set("username", c.User)
	q.Set("password", b64)

	endpoint := fmt.Sprintf("%s/%s?%s", strings.TrimRight(c.BaseURL, "/"), strings.TrimLeft(path, "/"), q.Encode())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, res, err
	}
	defer res.Body.Close()

	var out loginResp
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, res, err
	}
	return &out, res, nil
}

func (c *Client) Login(ctx context.Context) error {
	// ลอง primary endpoint ก่อน ตามสเปค
	if out, _, err := c.loginOnce(ctx, "LoginToService"); err == nil && out != nil {
		if out.Result == 0 && out.Token != "" {
			c.Token = out.Token
			return nil
		}
		// รวมรายละเอียดไว้ใน error เพื่อ debug
		return fmt.Errorf("login failed: result=%d errorcount=%d", out.Result, out.ErrorCount)
	}

	// เผื่อบางระบบใช้ชื่อ path อื่น เช่น LoginService
	if out, _, err := c.loginOnce(ctx, "LoginService"); err == nil && out != nil {
		if out.Result == 0 && out.Token != "" {
			c.Token = out.Token
			return nil
		}
		return fmt.Errorf("login failed (alt): result=%d errorcount=%d", out.Result, out.ErrorCount)
	}

	return fmt.Errorf("login failed: unreachable or unexpected response")
}

// --------- Read channels by page ---------

type ChannelItem struct {
	Valid      int    `json:"valid"`
	PID        int    `json:"pid"`
	PName      string `json:"pname"`
	Name       string `json:"name"`
	GBID       string `json:"gbid"`
	Longitude  string `json:"longitude"`
	Latitude   string `json:"latitude"`
	CameraType int    `json:"cameratype"`
	HLS        string `json:"hls"`
	RTSP       string `json:"rtsp"`
	HTTPFLV    string `json:"httpflv"`
	WS         string `json:"ws"`
	Channel    int    `json:"channel"`
}

type ChannelsPage struct {
	Result      int           `json:"result"`
	PageNo      int           `json:"pageNo"`
	PageSize    int           `json:"pageSize"`
	PageCount   int           `json:"pageCount"`
	RecordCount int           `json:"recordCount"`
	Items       []ChannelItem `json:"items"`
}

// internal/gateways/svms/svmsclient.go
func (c *Client) ReadChannelListByPage(ctx context.Context, pageNo, pageSize int, withRTSP bool) (*ChannelsPage, error) {
	q := url.Values{}
	q.Set("pageNo", strconv.Itoa(pageNo))
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("hls", "0")
	if withRTSP {
		q.Set("rtsp", "1")
	}
	q.Set("httpflv", "0")
	q.Set("ws", "0")
	if c.Token != "" {
		q.Set("token", c.Token)
	} // << สำคัญ

	endpoint := fmt.Sprintf("%s/org/ReadChannelListByPage?%s", strings.TrimRight(c.BaseURL, "/"), q.Encode())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// ป้องกันกรณีคืน non-JSON เช่น "Unauthorized"
	ct := res.Header.Get("Content-Type")
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20)) // 1MB safety
	if !strings.Contains(strings.ToLower(ct), "application/json") {
		// ลอง decode JSON อยู่ดี เผื่อบางระบบลืมใส่ content-type
		var out ChannelsPage
		if err := json.Unmarshal(body, &out); err != nil {
			snip := string(body)
			if len(snip) > 200 {
				snip = snip[:200] + "..."
			}
			return nil, fmt.Errorf("SVMS non-JSON response: %s", snip)
		}
		return &out, nil
	}

	var out ChannelsPage
	if err := json.Unmarshal(body, &out); err != nil {
		snip := string(body)
		if len(snip) > 200 {
			snip = snip[:200] + "..."
		}
		return nil, fmt.Errorf("SVMS JSON decode failed: %v; body: %s", err, snip)
	}
	if out.Result != 0 {
		return nil, fmt.Errorf("SVMS page fetch failed: result=%d", out.Result)
	}
	return &out, nil
}

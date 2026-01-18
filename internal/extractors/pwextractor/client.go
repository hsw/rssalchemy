package pwextractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type flareClient struct {
	baseURL    string
	httpClient *http.Client
}

type flareProxy struct {
	Url      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type flareCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type flareRequest struct {
	Cmd              string        `json:"cmd"`
	Url              string        `json:"url,omitempty"`
	Cookies          []flareCookie `json:"cookies,omitempty"`
	MaxTimeout       int           `json:"maxTimeout,omitempty"`
	Proxy            *flareProxy   `json:"proxy,omitempty"`
	Session          string        `json:"session,omitempty"`
	SessionTTL       int           `json:"session_ttl_minutes,omitempty"`
	ReturnOnlyCookies bool         `json:"returnOnlyCookies,omitempty"`
	ReturnScreenshot bool          `json:"returnScreenshot,omitempty"`
	WaitInSeconds    int           `json:"waitInSeconds,omitempty"`
	DisableMedia     bool          `json:"disableMedia,omitempty"`
}

type flareCookieResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type flareSolution struct {
	Url        string                `json:"url"`
	Status     int                   `json:"status"`
	Headers    map[string]string     `json:"headers"`
	Response   string                `json:"response"`
	Cookies    []flareCookieResponse `json:"cookies"`
	UserAgent  string                `json:"userAgent"`
	Screenshot string                `json:"screenshot"`
}

type flareResponse struct {
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Session  string         `json:"session"`
	Solution *flareSolution `json:"solution"`
}

func newFlareClient(baseURL string, maxTimeoutMs int) (*flareClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("flaresolverr url is empty")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	timeout := time.Duration(maxTimeoutMs+5000) * time.Millisecond
	return &flareClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *flareClient) do(ctx context.Context, req flareRequest) (*flareResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal flaresolverr request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create flaresolverr request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr request failed: %w", err)
	}
	defer resp.Body.Close()

	var decoded flareResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode flaresolverr response: %w", err)
	}
	if decoded.Status != "ok" {
		msg := decoded.Message
		if msg == "" {
			msg = fmt.Sprintf("status %s", decoded.Status)
		}
		return nil, fmt.Errorf("flaresolverr error: %s", msg)
	}
	return &decoded, nil
}

func (c *flareClient) createSession(ctx context.Context, proxy *flareProxy) (string, error) {
	req := flareRequest{
		Cmd:   "sessions.create",
		Proxy: proxy,
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return "", err
	}
	if resp.Session == "" {
		return "", fmt.Errorf("empty session id")
	}
	return resp.Session, nil
}

func (c *flareClient) destroySession(ctx context.Context, session string) error {
	req := flareRequest{
		Cmd:     "sessions.destroy",
		Session: session,
	}
	_, err := c.do(ctx, req)
	return err
}

func toFlareCookies(cookies [][2]string) []flareCookie {
	out := make([]flareCookie, 0, len(cookies))
	for _, cook := range cookies {
		out = append(out, flareCookie{Name: cook[0], Value: cook[1]})
	}
	return out
}


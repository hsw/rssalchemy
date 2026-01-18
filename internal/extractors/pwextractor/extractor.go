package pwextractor

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/egor3f/rssalchemy/internal/limiter"
	"github.com/egor3f/rssalchemy/internal/models"
	"github.com/labstack/gommon/log"
	"net"
	"time"
)

type DateParser interface {
	ParseDate(string) (time.Time, error)
}

type CookieManager interface {
	GetCookies(key string, cookieHeader string) ([][2]string, error)
	UpdateCookies(key string, cookieHeader string, cookies [][2]string) error
}

type PwExtractor struct {
	client        *flareClient
	dateParser    DateParser
	cookieManager CookieManager
	limiter       limiter.Limiter
	proxy         *flareProxy
	proxyHasAuth  bool
	proxyIP       net.IP
	maxTimeoutMs  int
	waitSeconds   int
}

type Config struct {
	Proxy                 string
	DateParser            DateParser
	CookieManager         CookieManager
	Limiter               limiter.Limiter
	FlareSolverrURL        string
	FlareSolverrMaxTimeout int
	FlareSolverrWait       int
}

const (
	defaultMaxTimeoutMs = 60000
)

func New(cfg Config) (*PwExtractor, error) {
	if cfg.DateParser == nil || cfg.CookieManager == nil || cfg.Limiter == nil {
		panic("you fckd up with di again")
	}

	proxy, proxyHasAuth, proxyHost, err := parseProxy(cfg.Proxy)
	if err != nil {
		return nil, fmt.Errorf("parse proxy: %w", err)
	}

	var proxyIP net.IP
	if proxyHost != "" {
		proxyIPs, err := getIPs(proxyHost)
		if err != nil {
			return nil, fmt.Errorf("get proxy ip: %w", err)
		}
		proxyIP = proxyIPs[0]
	}

	maxTimeoutMs := cfg.FlareSolverrMaxTimeout
	if maxTimeoutMs <= 0 {
		maxTimeoutMs = defaultMaxTimeoutMs
	}

	client, err := newFlareClient(cfg.FlareSolverrURL, maxTimeoutMs)
	if err != nil {
		return nil, fmt.Errorf("create flaresolverr client: %w", err)
	}

	return &PwExtractor{
		client:        client,
		dateParser:    cfg.DateParser,
		cookieManager: cfg.CookieManager,
		limiter:       cfg.Limiter,
		proxy:         proxy,
		proxyHasAuth:  proxyHasAuth,
		proxyIP:       proxyIP,
		maxTimeoutMs:  maxTimeoutMs,
		waitSeconds:   cfg.FlareSolverrWait,
	}, nil
}

func (e *PwExtractor) Stop() error {
	return nil
}

func (e *PwExtractor) Extract(task models.Task) (result *models.TaskResult, errRet error) {
	solution, baseURL, err := e.fetchSolution(context.Background(), task, false)
	if err != nil {
		return nil, err
	}

	parser := htmlParser{
		task:       task,
		dateParser: e.dateParser,
		baseURL:    baseURL,
	}
	result, err = parser.parse(solution.Response)
	if err != nil {
		return nil, fmt.Errorf("parse page: %w", err)
	}
	return result, nil
}

func (e *PwExtractor) Screenshot(task models.Task) (result *models.ScreenshotTaskResult, errRet error) {
	solution, _, err := e.fetchSolution(context.Background(), task, true)
	if err != nil {
		return nil, err
	}
	if solution.Screenshot == "" {
		return nil, fmt.Errorf("empty screenshot payload")
	}
	image, err := base64.StdEncoding.DecodeString(solution.Screenshot)
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}
	result = &models.ScreenshotTaskResult{Image: image}
	return result, nil
}

func (e *PwExtractor) fetchSolution(ctx context.Context, task models.Task, wantScreenshot bool) (*flareSolution, *urlParts, error) {
	baseDomain, _, err := parseBaseDomain(task.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse base domain: %w", err)
	}

	waitFor, err := e.limiter.Limit(ctx, baseDomain)
	if err != nil {
		return nil, nil, fmt.Errorf("bydomain limiter: %w", err)
	}
	if waitFor > 0 {
		log.Infof("Bydomain limiter domain=%s wait=%v", baseDomain, waitFor)
		time.Sleep(waitFor)
	}

	allowHost, err := e.allowHost(task.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("allow host: %w", err)
	}
	if !allowHost {
		return nil, nil, fmt.Errorf("blocked host: %s", task.URL)
	}

	cookieStr, cookies := e.extractCookies(task.Headers, task.URL)

	req := flareRequest{
		Cmd:              "request.get",
		Url:              task.URL,
		MaxTimeout:       e.maxTimeoutMs,
		ReturnScreenshot: wantScreenshot,
		WaitInSeconds:    e.waitSeconds,
	}
	if len(cookies) > 0 {
		req.Cookies = toFlareCookies(cookies)
	}
	if e.proxy != nil {
		if e.proxyHasAuth {
			session, err := e.client.createSession(ctx, e.proxy)
			if err != nil {
				return nil, nil, fmt.Errorf("create session: %w", err)
			}
			defer func() {
				if err := e.client.destroySession(ctx, session); err != nil {
					log.Warnf("destroy session failed: %v", err)
				}
			}()
			req.Session = session
		} else {
			req.Proxy = e.proxy
		}
	}

	resp, err := e.client.do(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	if resp.Solution == nil {
		return nil, nil, fmt.Errorf("empty flaresolverr solution")
	}
	if resp.Solution.Url != "" {
		allowHost, err := e.allowHost(resp.Solution.Url)
		if err != nil {
			return nil, nil, fmt.Errorf("allow host: %w", err)
		}
		if !allowHost {
			return nil, nil, fmt.Errorf("blocked host: %s", resp.Solution.Url)
		}
	}

	baseURL := parseURL(task.URL)
	if parsed := parseURL(resp.Solution.Url); parsed != nil {
		baseURL = parsed
	}

	if len(cookies) > 0 {
		newCookies := make([][2]string, 0, len(resp.Solution.Cookies))
		for _, cook := range resp.Solution.Cookies {
			newCookies = append(newCookies, [2]string{cook.Name, cook.Value})
		}
		if err := e.cookieManager.UpdateCookies(task.URL, cookieStr, newCookies); err != nil {
			log.Errorf("cookie manager update: %v", err)
		}
	}

	return resp.Solution, baseURL, nil
}

func (e *PwExtractor) extractCookies(headers map[string]string, taskURL string) (string, [][2]string) {
	cookieStr := ""
	cookies := make([][2]string, 0)
	if headers == nil {
		return cookieStr, cookies
	}
	if v, ok := headers["Cookie"]; ok && len(v) > 0 {
		cookieStr = v
		found, err := e.cookieManager.GetCookies(taskURL, v)
		if err != nil {
			log.Errorf("cookie manager get: %v", err)
		} else {
			cookies = found
		}
		log.Debugf("Found cookies, count=%d", len(cookies))
	}
	return cookieStr, cookies
}

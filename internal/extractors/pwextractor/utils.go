package pwextractor

import (
	"fmt"
	"github.com/jellydator/ttlcache/v3"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
)

type urlParts = url.URL

func parseURL(raw string) *urlParts {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return parsed
}

func absURL(link string, baseURL *urlParts) string {
	if strings.TrimSpace(link) == "" {
		return ""
	}
	if baseURL == nil {
		return link
	}
	parsed, err := url.Parse(link)
	if err != nil {
		return link
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	return baseURL.ResolveReference(parsed).String()
}

func parseProxy(s string) (*flareProxy, bool, string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, false, "", nil
	}
	proxyUrl, err := url.Parse(s)
	if err != nil {
		return nil, false, "", err
	}
	urlWithoutUser := *proxyUrl
	urlWithoutUser.User = nil
	proxy := &flareProxy{Url: urlWithoutUser.String()}

	hasAuth := false
	if proxyUrl.User != nil {
		proxy.Username = proxyUrl.User.Username()
		if pass, exist := proxyUrl.User.Password(); exist {
			proxy.Password = pass
			hasAuth = true
		}
		if proxy.Username != "" {
			hasAuth = true
		}
	}
	return proxy, hasAuth, proxyUrl.Hostname(), nil
}

// parseBaseDomain extracts second-level domain from url, e.g.
// https://kek.example.com/lol becomes example.com
// if url is invalid or scheme is not http(s), returns error, otherwise returns scheme and domain
func parseBaseDomain(urlStr string) (domain string, scheme string, err error) {
	pageUrl, err := url.Parse(urlStr)
	if err != nil {
		return "", "", fmt.Errorf("task url parsing: %w", err)
	}
	scheme = pageUrl.Scheme
	if !slices.Contains([]string{"https", "http"}, scheme) {
		return "", "", fmt.Errorf("bad scheme: %s", scheme)
	}
	hostname := strings.ToLower(pageUrl.Hostname())
	ipHost := net.ParseIP(hostname)
	if ipHost != nil {
		return ipHost.String(), scheme, nil
	}
	domainParts := strings.Split(hostname, ".")
	slices.Reverse(domainParts) // com, example, www
	return fmt.Sprintf("%s.%s", domainParts[1], domainParts[0]), scheme, nil
}

var dnsCache *ttlcache.Cache[string, []net.IP]

func init() {
	dnsCache = ttlcache.New[string, []net.IP](
		ttlcache.WithTTL[string, []net.IP](1*time.Minute),
		ttlcache.WithDisableTouchOnHit[string, []net.IP](),
	)
	go dnsCache.Start()
}

// getIPs from url, hostname, ip string
// result slice len always > 0 if error is nil
func getIPs(host string) ([]net.IP, error) {
	ip := net.ParseIP(host)
	if ip != nil {
		return []net.IP{ip}, nil
	}

	urlStruct, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("url parse: %w", err)
	}
	if len(urlStruct.Host) > 0 {
		host = urlStruct.Hostname()
		ip = net.ParseIP(host)
		if ip != nil {
			return []net.IP{ip}, nil
		}
	}

	var ips []net.IP
	if dnsCache.Has(host) {
		ips = dnsCache.Get(host).Value()
	} else {
		ips, err = net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("lookup ip: %w", err)
		}
		dnsCache.Set(host, ips, ttlcache.DefaultTTL)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("lookip ip: not resolved")
	}
	return ips, nil
}

func (e *PwExtractor) allowHost(rawUrl string) (bool, error) {
	ips, err := getIPs(rawUrl)
	if err != nil {
		return false, fmt.Errorf("allow host get ips: %w", err)
	}
	for _, ip := range ips {
		deny := ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast()
		if e.proxyIP != nil {
			deny = deny || e.proxyIP.Equal(ip)
		}
		if deny {
			return false, nil
		}
	}
	return true, nil
}

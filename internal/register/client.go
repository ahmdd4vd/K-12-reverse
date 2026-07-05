package register

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/tls-client"
	"github.com/google/uuid"

	"github.com/verssache/chatgpt-creator/internal/chrome"
	"github.com/verssache/chatgpt-creator/internal/util"
)

const (
	baseURL = "https://chatgpt.com"
	authURL = "https://auth.openai.com"
)

type Client struct {
	session     tls_client.HttpClient
	proxy       string
	tag         string
	workerID    int
	deviceID    string
	impersonate string
	major       int
	fullVersion string
	ua          string
	secChUA     string
	printMu     *sync.Mutex
	fileMu      *sync.Mutex
}

func NewClient(proxy, tag string, workerID int, printMu, fileMu *sync.Mutex) (*Client, error) {
	profile, fullVersion, ua := chrome.RandomChromeVersion()
	impersonate := profile.Impersonate
	mappedProfile := chrome.MapToTLSProfile(impersonate)

	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(mappedProfile),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
	}

	if proxy != "" {
		formattedProxy := util.FormatProxy(proxy)
		options = append(options, tls_client.WithProxyUrl(formattedProxy))
	}

	session, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create http client: %w", err)
	}

	deviceID := uuid.New().String()

	c := &Client{
		session:     session,
		proxy:       proxy,
		tag:         tag,
		workerID:    workerID,
		deviceID:    deviceID,
		impersonate: impersonate,
		fullVersion: fullVersion,
		ua:          ua,
		printMu:     printMu,
		fileMu:      fileMu,
	}

	// major version for sec-ch-ua
	c.major = profile.Major
	c.secChUA = profile.SecChUA

	// Add initial cookie
	u, _ := url.Parse(baseURL)
	cookies := []*http.Cookie{
		{
			Name:   "oai-did",
			Value:  deviceID,
			Domain: "chatgpt.com",
			Path:   "/",
		},
	}
	session.GetCookieJar().SetCookies(u, cookies)

	return c, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.ua)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
	if req.Header.Get("Accept-Language") == "" {
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	}
	if req.Header.Get("sec-ch-ua") == "" {
		req.Header.Set("sec-ch-ua", c.secChUA)
	}
	if req.Header.Get("sec-ch-ua-mobile") == "" {
		req.Header.Set("sec-ch-ua-mobile", "?0")
	}
	if req.Header.Get("sec-ch-ua-platform") == "" {
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	}

	return c.session.Do(req)
}

func (c *Client) log(step string, status int) {
	c.printMu.Lock()
	defer c.printMu.Unlock()

	ts := time.Now().Format("15:04:05")
	fmt.Printf("[%s] [W%d] [%s] %s | %d\n", ts, c.workerID, c.tag, step, status)
}

func (c *Client) print(msg string) {
	c.printMu.Lock()
	defer c.printMu.Unlock()

	ts := time.Now().Format("15:04:05")
	fmt.Printf("[%s] [W%d] [%s] %s\n", ts, c.workerID, c.tag, msg)
}

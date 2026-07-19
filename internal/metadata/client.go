package metadata

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponseBytes = 8 << 20

type Container struct {
	HostUUID   string `json:"host_uuid"`
	ExternalID string `json:"external_id"`
}

type Host struct {
	UUID string `json:"uuid"`
}

type Client interface {
	GetContainers(context.Context) ([]Container, error)
	GetSelfHost(context.Context) (Host, error)
}

type HTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPClient(rawURL, caRootPath string) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid metadata URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("metadata URL must not contain credentials, query, or fragment")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caRootPath != "" {
		pem, err := os.ReadFile(caRootPath)
		if err != nil {
			return nil, fmt.Errorf("read metadata CA root: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("metadata CA root contains no certificates")
		}
		transport.TLSClientConfig.RootCAs = roots
	}

	return &HTTPClient{
		baseURL: parsed.String(),
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 3 || request.URL.Host != parsed.Host {
					return fmt.Errorf("metadata redirect rejected")
				}
				return nil
			},
		},
	}, nil
}

func (client *HTTPClient) get(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("metadata request %s returned %s", path, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("metadata response %s exceeds %d bytes", path, maxResponseBytes)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode metadata response %s: %w", path, err)
	}
	return nil
}

func (client *HTTPClient) GetContainers(ctx context.Context) (containers []Container, err error) {
	err = client.get(ctx, "/containers", &containers)
	return
}

func (client *HTTPClient) GetSelfHost(ctx context.Context) (host Host, err error) {
	err = client.get(ctx, "/self/host", &host)
	return
}

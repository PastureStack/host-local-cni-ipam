package metadata

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxResponseBytes       = 8 << 20
	defaultMetadataHost    = "169.254.169.250"
	alternateMetadataHost  = "169.254.169.251"
	metadataVersionPath    = "/2016-07-29"
	metadataContainersPath = metadataVersionPath + "/containers"
	metadataSelfHostPath   = metadataVersionPath + "/self/host"
	metadataCARootPath     = "/var/lib/pasturestack/etc/ssl/ca.crt"

	// DefaultMetadataURL is the fixed link-local endpoint used by the
	// supported platform deployment contract.
	DefaultMetadataURL = "http://" + defaultMetadataHost + metadataVersionPath
)

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

type metadataOriginPolicy struct {
	origin string
}

type policyTransport struct {
	base   http.RoundTripper
	policy *metadataOriginPolicy
}

func (transport *policyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.Method != http.MethodGet {
		return nil, fmt.Errorf("metadata request is invalid")
	}
	if transport == nil || transport.base == nil || transport.policy == nil || !isValidMetadataRequestURL(request.URL.String(), transport.policy) {
		return nil, fmt.Errorf("metadata request destination is not authorized")
	}
	return transport.base.RoundTrip(request)
}

type HTTPClient struct {
	baseURL string
	policy  *metadataOriginPolicy
	client  *http.Client
}

func NewHTTPClient(rawURL, caRootPath string) (*HTTPClient, error) {
	baseURL, policy, err := approvedMetadataBaseURL(rawURL)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Platform metadata is link-local. Ambient proxy settings must never
	// redirect this privileged node request outside the host network boundary.
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caRootPath != "" {
		pem, err := readApprovedMetadataCARoot(caRootPath)
		if err != nil {
			return nil, err
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
		baseURL: baseURL,
		policy:  policy,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &policyTransport{base: transport, policy: policy},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func approvedMetadataBaseURL(rawURL string) (string, *metadataOriginPolicy, error) {
	if rawURL == "" || rawURL != strings.TrimSpace(rawURL) || strings.ContainsAny(rawURL, "\r\n\t") {
		return "", nil, fmt.Errorf("invalid metadata URL")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.Hostname() == "" {
		return "", nil, fmt.Errorf("invalid metadata URL")
	}
	if parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", nil, fmt.Errorf("metadata URL must not contain credentials, encoded path, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != metadataVersionPath {
		return "", nil, fmt.Errorf("metadata URL must use the supported API path")
	}
	if !isApprovedMetadataHost(parsed.Hostname()) {
		return "", nil, fmt.Errorf("metadata URL must use a reserved platform metadata address")
	}
	if !usesStandardPort(parsed) {
		return "", nil, fmt.Errorf("metadata URL must use its scheme's standard port")
	}
	origin, err := canonicalMetadataOrigin(parsed)
	if err != nil {
		return "", nil, err
	}
	policy := &metadataOriginPolicy{origin: origin}
	baseURL := parsed.String()
	if !isValidMetadataRequestURL(baseURL+"/containers", policy) || !isValidMetadataRequestURL(baseURL+"/self/host", policy) {
		return "", nil, fmt.Errorf("metadata request destination is not authorized")
	}
	return baseURL, policy, nil
}

func readApprovedMetadataCARoot(requestedPath string) ([]byte, error) {
	requestedPath = strings.TrimSpace(requestedPath)
	if requestedPath != metadataCARootPath {
		return nil, fmt.Errorf("metadata CA root must use %s", metadataCARootPath)
	}
	// The filesystem sink receives a constant managed mount path. CNI input and
	// process environment can opt in, but cannot select an arbitrary host file.
	pem, err := os.ReadFile(metadataCARootPath)
	if err != nil {
		return nil, fmt.Errorf("read metadata CA root: %w", err)
	}
	return pem, nil
}

func canonicalMetadataOrigin(parsed *url.URL) (string, error) {
	if parsed == nil {
		return "", fmt.Errorf("metadata URL is missing")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("metadata URL must use http or https")
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" {
		return "", fmt.Errorf("metadata URL must include a host")
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("metadata URL contains an invalid port")
	}
	return scheme + "://" + net.JoinHostPort(hostname, port), nil
}

func isApprovedMetadataHost(hostname string) bool {
	address := net.ParseIP(hostname)
	if address == nil || address.To4() == nil {
		return false
	}
	canonical := address.String()
	if hostname != canonical {
		return false
	}
	return canonical == defaultMetadataHost || canonical == alternateMetadataHost
}

func usesStandardPort(parsed *url.URL) bool {
	if parsed == nil || parsed.Port() == "" {
		return true
	}
	return (strings.EqualFold(parsed.Scheme, "http") && parsed.Port() == "80") ||
		(strings.EqualFold(parsed.Scheme, "https") && parsed.Port() == "443")
}

func isValidMetadataRequestURL(rawURL string, policy *metadataOriginPolicy) bool {
	if policy == nil {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.RawPath != "" ||
		parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Path != metadataContainersPath && parsed.Path != metadataSelfHostPath {
		return false
	}
	origin, err := canonicalMetadataOrigin(parsed)
	return err == nil && origin == policy.origin
}

func (client *HTTPClient) get(ctx context.Context, path string, target any) error {
	if client == nil || client.client == nil || client.policy == nil {
		return fmt.Errorf("metadata client is not configured")
	}
	if path != "/containers" && path != "/self/host" {
		return fmt.Errorf("metadata resource is not authorized")
	}
	requestURL := client.baseURL + path
	if !isValidMetadataRequestURL(requestURL, client.policy) {
		return fmt.Errorf("metadata request destination is not authorized")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
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

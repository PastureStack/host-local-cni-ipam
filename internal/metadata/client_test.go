package metadata

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

type recordingTransport struct {
	requests int
}

func (transport *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests++
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`[]`)),
		Request:    request,
	}, nil
}

func TestClientAcceptsSupportedLinkLocalMetadataOrigins(t *testing.T) {
	for _, rawURL := range []string{
		"http://169.254.169.250/2016-07-29",
		"https://169.254.169.251/2016-07-29/",
	} {
		client, err := NewHTTPClient(rawURL, "")
		if err != nil {
			t.Fatalf("expected supported metadata origin %q: %v", rawURL, err)
		}
		if client.baseURL == "" || client.policy == nil || client.client == nil {
			t.Fatalf("metadata origin %q produced an incomplete client", rawURL)
		}
	}
}

func TestClientRejectsUnapprovedMetadataOrigins(t *testing.T) {
	credentialURL := "http://user" + ":" + "pass@169.254.169.250/2016-07-29"
	for _, rawURL := range []string{
		"metadata/2016-07-29",
		"file:///2016-07-29",
		"http://127.0.0.1/2016-07-29",
		"http://169.254.169.254/latest/meta-data",
		"http://169.254.169.250:8080/2016-07-29",
		"https://169.254.169.250:8443/2016-07-29",
		"http://metadata/2016-07-29",
		credentialURL,
		"http://169.254.169.250/2016-07-29?token=value",
		"http://169.254.169.250/2016-07-29?",
		"http://169.254.169.250/2016-07-29#fragment",
		"http://169.254.169.250/2016-07-29%2fcontainers",
		"http://[::ffff:169.254.169.250]/2016-07-29",
		" http://169.254.169.250/2016-07-29",
		"http://169.254.169.250/2016-07-29\n",
	} {
		if _, err := NewHTTPClient(rawURL, ""); err == nil {
			t.Fatalf("expected unapproved metadata origin %q to fail", rawURL)
		}
	}
}

func TestClientRejectsUnapprovedCARootPath(t *testing.T) {
	path := t.TempDir() + "/metadata-ca.pem"
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHTTPClient("https://169.254.169.250/2016-07-29", path); err == nil || !strings.Contains(err.Error(), "must use") {
		t.Fatalf("expected arbitrary CA root path to fail, got %v", err)
	}
	if _, err := readApprovedMetadataCARoot(metadataCARootPath); err == nil || !strings.Contains(err.Error(), "read metadata CA root") {
		t.Fatalf("expected missing managed CA root to reach only its fixed path, got %v", err)
	}
}

func TestPolicyTransportEnforcesDestinationAtNetworkBoundary(t *testing.T) {
	base := &recordingTransport{}
	policy := &metadataOriginPolicy{origin: "http://169.254.169.250:80"}
	transport := &policyTransport{base: base, policy: policy}
	client := &http.Client{Transport: transport}
	for _, denied := range []string{
		"http://169.254.169.251" + metadataContainersPath,
		"https://169.254.169.250" + metadataContainersPath,
		"http://169.254.169.250:8080" + metadataContainersPath,
		"http://169.254.169.250/latest/meta-data",
	} {
		if _, err := client.Get(denied); err == nil {
			t.Fatalf("changed metadata destination %q was accepted", denied)
		}
	}
	if base.requests != 0 {
		t.Fatalf("unauthorized requests reached the network %d times", base.requests)
	}
	response, err := client.Get("http://169.254.169.250" + metadataContainersPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if base.requests != 1 {
		t.Fatalf("approved request reached the network %d times", base.requests)
	}
}

func TestPolicyTransportRejectsInvalidRequestShape(t *testing.T) {
	base := &recordingTransport{}
	policy := &metadataOriginPolicy{origin: "http://169.254.169.250:80"}
	transport := &policyTransport{base: base, policy: policy}
	if _, err := transport.RoundTrip(nil); err == nil {
		t.Fatal("expected nil request to fail")
	}
	request, err := http.NewRequest(http.MethodGet, "http://169.254.169.250"+metadataContainersPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&policyTransport{policy: policy}).RoundTrip(request); err == nil {
		t.Fatal("expected a transport without its proxy-disabled base to fail")
	}
	request, err = http.NewRequest(http.MethodPost, "http://169.254.169.250"+metadataContainersPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("expected non-GET metadata request to fail")
	}
	if base.requests != 0 {
		t.Fatalf("invalid requests reached the network %d times", base.requests)
	}
}

func TestClientReadsCleanupResources(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(metadataContainersPath, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`[{"host_uuid":"host-a","external_id":"container-a"}]`))
	})
	mux.HandleFunc(metadataSelfHostPath, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"uuid":"host-a"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := newTestHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	containers, err := client.GetContainers(context.Background())
	if err != nil || len(containers) != 1 || containers[0].ExternalID != "container-a" {
		t.Fatalf("containers = %#v, err = %v", containers, err)
	}
	host, err := client.GetSelfHost(context.Background())
	if err != nil || host.UUID != "host-a" {
		t.Fatalf("host = %#v, err = %v", host, err)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer redirector.Close()
	client, err := newTestHTTPClient(redirector.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetContainers(context.Background()); err == nil {
		t.Fatal("expected redirect response to fail")
	}
	if redirected.Load() {
		t.Fatal("metadata request followed a redirect")
	}
}

func TestClientRejectsOversizedResponseAndInvalidState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`[]` + strings.Repeat(" ", maxResponseBytes)))
	}))
	defer server.Close()
	client, err := newTestHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetContainers(context.Background()); err == nil {
		t.Fatal("expected oversized response to fail")
	}

	var nilClient *HTTPClient
	if _, err := nilClient.GetContainers(context.Background()); err == nil {
		t.Fatal("expected nil metadata client to fail")
	}
	client.baseURL = "http://169.254.169.251/2016-07-29"
	if _, err := client.GetContainers(context.Background()); err == nil {
		t.Fatal("expected a mutated metadata destination to fail before the network")
	}
}

func TestCanonicalMetadataOriginRejectsInvalidValues(t *testing.T) {
	for _, rawURL := range []string{
		"ftp://169.254.169.250/2016-07-29",
		"http:///2016-07-29",
		"http://169.254.169.250:70000/2016-07-29",
	} {
		parsed, _ := url.Parse(rawURL)
		if _, err := canonicalMetadataOrigin(parsed); err == nil {
			t.Fatalf("expected invalid origin %q to fail", rawURL)
		}
	}
	if _, err := canonicalMetadataOrigin(nil); err == nil {
		t.Fatal("expected nil origin to fail")
	}
}

func newTestHTTPClient(rawURL string) (*HTTPClient, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	origin, err := canonicalMetadataOrigin(parsed)
	if err != nil {
		return nil, err
	}
	policy := &metadataOriginPolicy{origin: origin}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &HTTPClient{
		baseURL: strings.TrimRight(rawURL, "/") + metadataVersionPath,
		policy:  policy,
		client: &http.Client{
			Transport: &policyTransport{base: transport, policy: policy},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

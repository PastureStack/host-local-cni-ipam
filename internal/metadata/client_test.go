package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientReadsCleanupResources(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/2016-07-29/containers", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`[{"host_uuid":"host-a","external_id":"container-a"}]`))
	})
	mux.HandleFunc("/2016-07-29/self/host", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"uuid":"host-a"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPClient(server.URL+"/2016-07-29", "")
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

func TestClientRejectsUnsafeURLAndOversizedResponse(t *testing.T) {
	for _, rawURL := range []string{"metadata/2016-07-29", "file:///metadata", "http://user@example.invalid"} {
		if _, err := NewHTTPClient(rawURL, ""); err == nil {
			t.Fatalf("expected %q to be rejected", rawURL)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`[]` + strings.Repeat(" ", maxResponseBytes)))
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetContainers(context.Background()); err == nil {
		t.Fatal("expected oversized response to fail")
	}
}

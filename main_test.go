package main

import (
	"context"
	"errors"
	"testing"

	platformmetadata "github.com/PastureStack/host-local-cni-ipam/internal/metadata"
)

type fakeMetadataClient struct {
	host       platformmetadata.Host
	containers []platformmetadata.Container
	err        error
}

func (client fakeMetadataClient) GetContainers(context.Context) ([]platformmetadata.Container, error) {
	return client.containers, client.err
}

func (client fakeMetadataClient) GetSelfHost(context.Context) (platformmetadata.Host, error) {
	return client.host, client.err
}

type fakeLeases struct {
	ids      []string
	released []string
}

func (leases *fakeLeases) GetAllContainers() ([]string, error) {
	return append([]string(nil), leases.ids...), nil
}

func (leases *fakeLeases) Release(id string) error {
	leases.released = append(leases.released, id)
	return nil
}

func TestCleanupKeepsOnlyContainersOnSelfHost(t *testing.T) {
	client := fakeMetadataClient{
		host: platformmetadata.Host{UUID: "host-a"},
		containers: []platformmetadata.Container{
			{HostUUID: "host-a", ExternalID: "keep"},
			{HostUUID: "host-b", ExternalID: "other-host"},
		},
	}
	active, err := activeContainerIDs(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	leases := &fakeLeases{ids: []string{"keep", "stale"}}
	removed, err := releaseStaleLeases(leases, active)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || len(leases.released) != 1 || leases.released[0] != "stale" {
		t.Fatalf("removed = %d, released = %#v", removed, leases.released)
	}
}

func TestCleanupRejectsMissingHostAndPropagatesMetadataFailure(t *testing.T) {
	if _, err := activeContainerIDs(context.Background(), fakeMetadataClient{}); err == nil {
		t.Fatal("expected missing host UUID to fail")
	}
	expected := errors.New("unavailable")
	if _, err := activeContainerIDs(context.Background(), fakeMetadataClient{err: expected}); !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
}

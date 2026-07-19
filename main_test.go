package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PastureStack/host-local-cni-ipam/allocator"
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

func TestMetadataURLPrecedenceUsesSupportedDefault(t *testing.T) {
	t.Setenv("PLATFORM_METADATA_URL", "http://169.254.169.251/2016-07-29")
	if got := metadataURL("http://169.254.169.250/2016-07-29"); got != "http://169.254.169.251/2016-07-29" {
		t.Fatalf("environment metadata URL = %q", got)
	}
	t.Setenv("PLATFORM_METADATA_URL", "")
	if got := metadataURL("http://169.254.169.251/2016-07-29"); got != "http://169.254.169.251/2016-07-29" {
		t.Fatalf("configured metadata URL = %q", got)
	}
	if got := metadataURL(""); got != "http://169.254.169.250/2016-07-29" {
		t.Fatalf("default metadata URL = %q", got)
	}
}

func TestVersionCommandIsExplicit(t *testing.T) {
	if !isVersionCommand([]string{"host-local-cni-ipam", "--version"}) {
		t.Fatal("expected the explicit version command to be accepted")
	}
	for _, args := range [][]string{
		nil,
		{"host-local-cni-ipam"},
		{"host-local-cni-ipam", "version"},
		{"host-local-cni-ipam", "--version", "extra"},
	} {
		if isVersionCommand(args) {
			t.Fatalf("unexpected version command: %#v", args)
		}
	}
}

func TestCommandLoggerRestrictsFileToManagedDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plugin.log")
	if err := os.WriteFile(path, []byte("existing"), 0o666); err != nil {
		t.Fatal(err)
	}
	file, err := openCommandLogFromRoot(root, path)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("log mode = %o", got)
		}
	}
	for _, unsafe := range []string{"", root, filepath.Join(root, "..", "outside.log"), filepath.Join("..", "outside.log")} {
		if file, err := openCommandLogFromRoot(root, unsafe); err == nil {
			_ = file.Close()
			t.Fatalf("expected unsafe log path %q to fail", unsafe)
		}
	}
}

func TestCommandLoggerUsesStandardErrorWithoutFile(t *testing.T) {
	logger, closeLog, err := commandLogger(&allocator.IPAMConfig{})
	if err != nil {
		t.Fatal(err)
	}
	closeLog()
	if logger == nil || logger.Writer() != os.Stderr {
		t.Fatal("expected the default logger to use standard error")
	}
}

func TestApprovedCommandLogPreservesDeployedPath(t *testing.T) {
	if filepath.ToSlash(filepath.Clean(legacyCNILogPath)) != "/var/log/pasturestack-cni.log" {
		t.Fatalf("deployed log path changed to %q", legacyCNILogPath)
	}
	if cniLogRoot != "/var/log/pasturestack" {
		t.Fatalf("managed log root changed to %q", cniLogRoot)
	}
}

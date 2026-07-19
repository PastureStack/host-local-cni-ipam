//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	platformmetadata "github.com/PastureStack/host-local-cni-ipam/internal/metadata"
	"github.com/containernetworking/cni/pkg/skel"
)

func TestCNIHandlersPreserveActiveAndRemoveStaleLeases(t *testing.T) {
	originalFactory := newMetadataClient
	defer func() { newMetadataClient = originalFactory }()
	newMetadataClient = func(rawURL, caRootPath string) (platformmetadata.Client, error) {
		if rawURL != platformmetadata.DefaultMetadataURL {
			t.Fatalf("metadata URL = %q", rawURL)
		}
		if caRootPath != "" {
			t.Fatalf("CA root = %q", caRootPath)
		}
		return fakeMetadataClient{
			host:       platformmetadata.Host{UUID: "host-a"},
			containers: []platformmetadata.Container{{HostUUID: "host-a", ExternalID: "keep"}},
		}, nil
	}

	config := []byte(fmt.Sprintf(`{"cniVersion":"1.1.0","name":"host-local-smoke","ipam":{"type":"host-local-cni-ipam","subnet":"192.0.2.0/29","dataDir":%q}}`, t.TempDir()))
	add := func(containerID string) string {
		t.Helper()
		output, err := captureStdout(func() error {
			return cmdAdd(&skel.CmdArgs{ContainerID: containerID, StdinData: config})
		})
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			IPs []struct {
				Address string `json:"address"`
			} `json:"ips"`
		}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("decode CNI output %q: %v", output, err)
		}
		if len(result.IPs) != 1 {
			t.Fatalf("CNI output = %s", output)
		}
		return result.IPs[0].Address
	}

	if got := add("keep"); got != "192.0.2.2/29" {
		t.Fatalf("first address = %q", got)
	}
	if got := add("stale"); got != "192.0.2.3/29" {
		t.Fatalf("second address = %q", got)
	}
	if err := cmdCheck(&skel.CmdArgs{ContainerID: "keep", StdinData: config}); err != nil {
		t.Fatal(err)
	}
	if err := cmdDel(&skel.CmdArgs{StdinData: config}); err != nil {
		t.Fatal(err)
	}
	if err := cmdCheck(&skel.CmdArgs{ContainerID: "keep", StdinData: config}); err != nil {
		t.Fatalf("active lease was removed: %v", err)
	}
	if err := cmdCheck(&skel.CmdArgs{ContainerID: "stale", StdinData: config}); err == nil {
		t.Fatal("stale lease was not removed")
	}
}

func TestCmdDelReleasesRequestedLeaseWhenMetadataIsUnavailable(t *testing.T) {
	originalFactory := newMetadataClient
	defer func() { newMetadataClient = originalFactory }()

	config := []byte(fmt.Sprintf(`{"cniVersion":"1.1.0","name":"host-local-outage","ipam":{"type":"host-local-cni-ipam","subnet":"198.51.100.0/29","dataDir":%q}}`, t.TempDir()))
	if _, err := captureStdout(func() error {
		return cmdAdd(&skel.CmdArgs{ContainerID: "delete-me", StdinData: config})
	}); err != nil {
		t.Fatal(err)
	}
	newMetadataClient = func(string, string) (platformmetadata.Client, error) {
		return fakeMetadataClient{err: errors.New("metadata unavailable")}, nil
	}
	if err := cmdDel(&skel.CmdArgs{ContainerID: "delete-me", StdinData: config}); err != nil {
		t.Fatal(err)
	}
	if err := cmdCheck(&skel.CmdArgs{ContainerID: "delete-me", StdinData: config}); err == nil {
		t.Fatal("requested lease remained after metadata outage")
	}
}

func captureStdout(run func() error) ([]byte, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = original
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if runErr != nil {
		return nil, runErr
	}
	return output, readErr
}

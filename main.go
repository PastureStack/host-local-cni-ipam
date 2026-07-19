// Copyright 2015 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PastureStack/host-local-cni-ipam/allocator"
	"github.com/PastureStack/host-local-cni-ipam/backend/disk"
	platformmetadata "github.com/PastureStack/host-local-cni-ipam/internal/metadata"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
)

const cniLogRoot = "/var/log/pasturestack"
const legacyCNILogPath = "/var/log/pasturestack-cni.log"

var buildVersion = "dev"

type leaseManager interface {
	GetAllContainers() ([]string, error)
	Release(string) error
}

var newMetadataClient = func(rawURL, caRootPath string) (platformmetadata.Client, error) {
	return platformmetadata.NewHTTPClient(rawURL, caRootPath)
}

func main() {
	if isVersionCommand(os.Args) {
		_, _ = fmt.Fprintln(os.Stdout, buildVersion)
		return
	}
	about := fmt.Sprintf("PastureStack host-local CNI IPAM %s", buildVersion)
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, about)
}

func isVersionCommand(args []string) bool {
	return len(args) == 2 && args[1] == "--version"
}

func cmdAdd(args *skel.CmdArgs) error {
	config, configVersion, err := allocator.LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return err
	}
	logger, closeLog, err := commandLogger(config)
	if err != nil {
		return err
	}
	defer closeLog()

	store, err := disk.New(config.Name, config.DataDir)
	if err != nil {
		return fmt.Errorf("create address store: %w", err)
	}
	defer store.Close()

	ipAllocator, err := allocator.NewIPAllocator(config, store)
	if err != nil {
		return err
	}
	allocation, err := ipAllocator.Get(args.ContainerID)
	if err != nil {
		return err
	}
	if config.IsDebugLevel == "true" {
		logger.Printf("allocated address for CNI container")
	}

	result := &current.Result{
		CNIVersion: current.ImplementedSpecVersion,
		IPs: []*current.IPConfig{{
			Address: allocation.Address,
			Gateway: allocation.Gateway,
		}},
		Routes: config.Routes,
	}
	return types.PrintResult(result, configVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	config, _, err := allocator.LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return err
	}
	store, err := disk.New(config.Name, config.DataDir)
	if err != nil {
		return fmt.Errorf("create address store: %w", err)
	}
	defer store.Close()
	if err := store.Lock(); err != nil {
		return err
	}
	defer func() { _ = store.Unlock() }()
	address, err := store.GetIPByID(args.ContainerID)
	if err != nil {
		return err
	}
	if address == nil {
		return fmt.Errorf("no address is allocated to the CNI container")
	}
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	config, _, err := allocator.LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return err
	}
	logger, closeLog, err := commandLogger(config)
	if err != nil {
		return err
	}
	defer closeLog()

	store, err := disk.New(config.Name, config.DataDir)
	if err != nil {
		return fmt.Errorf("create address store: %w", err)
	}
	defer store.Close()
	ipAllocator, err := allocator.NewIPAllocator(config, store)
	if err != nil {
		return err
	}

	if strings.TrimSpace(args.ContainerID) != "" {
		if err := ipAllocator.Release(args.ContainerID); err != nil {
			return fmt.Errorf("release requested CNI lease: %w", err)
		}
	}

	client, err := newMetadataClient(metadataURL(config.MetadataURL), os.Getenv("PLATFORM_CA_ROOT"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	activeIDs, err := activeContainerIDs(ctx, client)
	if err != nil {
		logger.Printf("metadata cleanup skipped: %v", err)
		return nil
	}
	removed, err := releaseStaleLeases(ipAllocator, activeIDs)
	if err != nil {
		return err
	}
	if config.IsDebugLevel == "true" {
		logger.Printf("removed %d stale address leases", removed)
	}
	return nil
}

func metadataURL(configured string) string {
	if value := strings.TrimSpace(os.Getenv("PLATFORM_METADATA_URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(configured); value != "" {
		return value
	}
	return platformmetadata.DefaultMetadataURL
}

func activeContainerIDs(ctx context.Context, client platformmetadata.Client) (map[string]struct{}, error) {
	host, err := client.GetSelfHost(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(host.UUID) == "" {
		return nil, fmt.Errorf("metadata self host has no UUID")
	}
	containers, err := client.GetContainers(ctx)
	if err != nil {
		return nil, err
	}
	active := map[string]struct{}{}
	for _, container := range containers {
		if container.HostUUID == host.UUID && strings.TrimSpace(container.ExternalID) != "" {
			active[container.ExternalID] = struct{}{}
		}
	}
	return active, nil
}

func releaseStaleLeases(leases leaseManager, active map[string]struct{}) (int, error) {
	persisted, err := leases.GetAllContainers()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, id := range persisted {
		if _, ok := active[id]; ok {
			continue
		}
		if err := leases.Release(id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func commandLogger(config *allocator.IPAMConfig) (*log.Logger, func(), error) {
	var output io.Writer = os.Stderr
	closeLog := func() {}
	if config.LogToFile != "" {
		file, err := openApprovedCommandLog(config.LogToFile)
		if err != nil {
			return nil, nil, err
		}
		output = file
		closeLog = func() { _ = file.Close() }
	}
	return log.New(output, "host-local-cni-ipam: ", log.LstdFlags), closeLog, nil
}

func openApprovedCommandLog(requestedPath string) (*os.File, error) {
	if filepath.Clean(strings.TrimSpace(requestedPath)) == legacyCNILogPath {
		return openCommandLogFromRoot(filepath.Dir(legacyCNILogPath), legacyCNILogPath)
	}
	return openCommandLogFromRoot(cniLogRoot, requestedPath)
}

func openCommandLogFromRoot(rootPath, requestedPath string) (*os.File, error) {
	requestedPath = strings.TrimSpace(requestedPath)
	if requestedPath == "" || strings.ContainsRune(requestedPath, '\x00') {
		return nil, fmt.Errorf("CNI log path is empty or invalid")
	}
	rootPath = filepath.Clean(rootPath)
	relativePath := filepath.Clean(requestedPath)
	if filepath.IsAbs(relativePath) {
		var err error
		relativePath, err = filepath.Rel(rootPath, relativePath)
		if err != nil {
			return nil, fmt.Errorf("CNI log path is outside the managed log directory")
		}
	}
	if relativePath == "." || !filepath.IsLocal(relativePath) {
		return nil, fmt.Errorf("CNI log path is outside the managed log directory")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open managed CNI log directory: %w", err)
	}
	defer root.Close()
	file, err := root.OpenFile(relativePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open CNI log file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("restrict CNI log file: %w", err)
	}
	return file, nil
}

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

package disk

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const lastIPFile = "last_reserved_ip"

const defaultDataDir = "/var/lib/cni/networks"

type Store struct {
	*FileLock
	dataDir string
}

func New(network, dataDir string) (*Store, error) {
	if network == "" || network == "." || network == ".." || strings.ContainsAny(network, `/\`) {
		return nil, fmt.Errorf("unsafe CNI network name %q", network)
	}
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	if !filepath.IsAbs(dataDir) {
		return nil, fmt.Errorf("CNI data directory must be absolute")
	}

	dir := filepath.Join(dataDir, network)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("CNI network data path is not a directory")
	}

	lock, err := NewFileLock(dir)
	if err != nil {
		return nil, err
	}
	return &Store{FileLock: lock, dataDir: dir}, nil
}

func (s *Store) Reserve(id string, ip net.IP) (bool, error) {
	filename := filepath.Join(s.dataDir, ip.String())
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0o600)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(filename)
	}
	if _, err := file.WriteString(strings.TrimSpace(id)); err != nil {
		cleanup()
		return false, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(filename)
		return false, err
	}
	if err := os.WriteFile(filepath.Join(s.dataDir, lastIPFile), []byte(ip.String()), 0o600); err != nil {
		_ = os.Remove(filename)
		return false, err
	}
	return true, nil
}

func (s *Store) LastReservedIP() (net.IP, error) {
	data, err := os.ReadFile(filepath.Join(s.dataDir, lastIPFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read last reserved IP: %w", err)
	}
	return net.ParseIP(strings.TrimSpace(string(data))), nil
}

func (s *Store) Release(ip net.IP) error {
	err := os.Remove(filepath.Join(s.dataDir, ip.String()))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) leaseFiles(visit func(path string, info os.FileInfo, value string) error) error {
	return filepath.Walk(s.dataDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || info.Name() == lastIPFile {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return visit(path, info, strings.TrimSpace(string(data)))
	})
}

func (s *Store) ReleaseByID(id string) error {
	match := strings.TrimSpace(id)
	return s.leaseFiles(func(path string, _ os.FileInfo, value string) error {
		if value != match {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

func (s *Store) GetIPByID(id string) (net.IP, error) {
	match := strings.TrimSpace(id)
	var address net.IP
	err := s.leaseFiles(func(_ string, info os.FileInfo, value string) error {
		if address == nil && value == match {
			address = net.ParseIP(info.Name())
		}
		return nil
	})
	return address, err
}

func (s *Store) GetAllIDs() ([]string, error) {
	unique := map[string]struct{}{}
	err := s.leaseFiles(func(_ string, _ os.FileInfo, value string) error {
		if value != "" {
			unique[value] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	return ids, nil
}

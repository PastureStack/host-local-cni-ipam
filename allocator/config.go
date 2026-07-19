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

package allocator

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/types"
)

// IPAMConfig represents the address-management part of a CNI configuration.
type IPAMConfig struct {
	Name         string
	Type         string         `json:"type"`
	RangeStart   net.IP         `json:"rangeStart"`
	RangeEnd     net.IP         `json:"rangeEnd"`
	Subnet       types.IPNet    `json:"subnet"`
	Gateway      net.IP         `json:"gateway"`
	Routes       []*types.Route `json:"routes"`
	DataDir      string         `json:"dataDir"`
	MetadataURL  string         `json:"metadataURL"`
	Args         *IPAMArgs      `json:"-"`
	IsDebugLevel string         `json:"isDebugLevel"`
	LogToFile    string         `json:"logToFile"`
}

type IPAMArgs struct {
	types.CommonArgs
	IP types.UnmarshallableString `json:"ip,omitempty"`
}

type Net struct {
	Name       string      `json:"name"`
	CNIVersion string      `json:"cniVersion"`
	IPAM       *IPAMConfig `json:"ipam"`
}

// LoadIPAMConfig creates an IPAM configuration from a CNI network document.
func LoadIPAMConfig(data []byte, args string) (*IPAMConfig, string, error) {
	n := Net{}
	if err := json.Unmarshal(data, &n); err != nil {
		return nil, "", err
	}
	if n.IPAM == nil {
		return nil, "", fmt.Errorf("IPAM config missing 'ipam' key")
	}
	if n.Name == "" {
		return nil, "", fmt.Errorf("network config missing 'name'")
	}

	if args != "" {
		n.IPAM.Args = &IPAMArgs{}
		if err := types.LoadArgs(args, n.IPAM.Args); err != nil {
			return nil, "", err
		}
		if value := string(n.IPAM.Args.IP); value != "" && net.ParseIP(value) == nil {
			return nil, "", fmt.Errorf("invalid requested IP address %q", value)
		}
	}
	if n.CNIVersion == "" {
		n.CNIVersion = "0.2.0"
	}

	n.IPAM.Name = n.Name
	return n.IPAM, n.CNIVersion, nil
}

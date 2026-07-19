// Copyright 2016 CNI authors
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
	"net"
	"testing"

	fakestore "github.com/PastureStack/host-local-cni-ipam/backend/testing"
	"github.com/containernetworking/cni/pkg/types"
)

func testConfig(t *testing.T, subnet string) *IPAMConfig {
	t.Helper()
	parsed, err := types.ParseCIDR(subnet)
	if err != nil {
		t.Fatal(err)
	}
	return &IPAMConfig{
		Name:   "test-network",
		Type:   "host-local-cni-ipam",
		Subnet: types.IPNet{IP: parsed.IP, Mask: parsed.Mask},
	}
}

func TestAllocatorUsesRoundRobinAndKeepsExistingLease(t *testing.T) {
	config := testConfig(t, "192.0.2.0/29")
	store := fakestore.NewFakeStore(map[string]string{"192.0.2.2": "first"}, net.ParseIP("192.0.2.2"))
	allocator, err := NewIPAllocator(config, store)
	if err != nil {
		t.Fatal(err)
	}
	allocation, err := allocator.Get("second")
	if err != nil {
		t.Fatal(err)
	}
	if got := allocation.Address.IP.String(); got != "192.0.2.3" {
		t.Fatalf("address = %s", got)
	}
	allocation, err = allocator.Get("second")
	if err != nil {
		t.Fatal(err)
	}
	if got := allocation.Address.IP.String(); got != "192.0.2.3" {
		t.Fatalf("existing address = %s", got)
	}
}

func TestAllocatorHonorsRequestedAddressAndRange(t *testing.T) {
	config := testConfig(t, "192.0.2.0/24")
	config.RangeStart = net.ParseIP("192.0.2.20")
	config.RangeEnd = net.ParseIP("192.0.2.30")
	config.Args = &IPAMArgs{IP: types.UnmarshallableString("192.0.2.25")}
	allocator, err := NewIPAllocator(config, fakestore.NewFakeStore(map[string]string{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	allocation, err := allocator.Get("requested")
	if err != nil {
		t.Fatal(err)
	}
	if got := allocation.Address.IP.String(); got != "192.0.2.25" {
		t.Fatalf("address = %s", got)
	}

	config.Args.IP = types.UnmarshallableString("192.0.2.31")
	allocator, err = NewIPAllocator(config, fakestore.NewFakeStore(map[string]string{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocator.Get("outside"); err == nil {
		t.Fatal("expected out-of-range request to fail")
	}
}

func TestAllocatorDoesNotLoopWhenRangeContainsOnlyGateway(t *testing.T) {
	config := testConfig(t, "192.0.2.0/29")
	config.RangeStart = net.ParseIP("192.0.2.1")
	config.RangeEnd = net.ParseIP("192.0.2.1")
	allocator, err := NewIPAllocator(config, fakestore.NewFakeStore(map[string]string{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocator.Get("none"); err == nil {
		t.Fatal("expected exhausted gateway-only range to fail")
	}
}

func TestLoadIPAMConfigValidatesStructureAndRequestedAddress(t *testing.T) {
	if _, _, err := LoadIPAMConfig([]byte(`{"name":"test"}`), "IP=192.0.2.10"); err == nil {
		t.Fatal("expected missing IPAM object to fail")
	}
	data := []byte(`{"cniVersion":"1.1.0","name":"test","ipam":{"type":"host-local-cni-ipam","subnet":"192.0.2.0/24"}}`)
	config, version, err := LoadIPAMConfig(data, "IP=192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.1.0" || string(config.Args.IP) != "192.0.2.10" {
		t.Fatalf("version = %q, args = %#v", version, config.Args)
	}
	if _, _, err := LoadIPAMConfig(data, "IP=not-an-address"); err == nil {
		t.Fatal("expected invalid requested address to fail")
	}
}

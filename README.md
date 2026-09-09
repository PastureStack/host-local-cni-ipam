# Host-local CNI IPAM

`host-local-cni-ipam` is a Linux CNI address-management plugin for allocating addresses from a host-local subnet. It keeps the historical round-robin lease behavior and can remove orphaned leases by comparing its local store with the platform metadata service.

PastureStack is an independent community effort to preserve, audit, and modernize the Rancher 1.6 ecosystem. It is not affiliated with or endorsed by Rancher Labs or SUSE.

**Upstream:** [`rancher/rancher-host-local-ipam`](https://github.com/rancher/rancher-host-local-ipam). This GitHub fork retains the upstream Git history, authorship, dates, and license notices unchanged; PastureStack maintenance is consolidated into one commit after the preserved upstream boundary.

The current public compatibility release is `v0.1.4`. This repository does
not publish a mutable `latest` tag; future releases must keep the same pure
numeric version format.

## POC scope

The local proof of concept supports:

- CNI versions 0.1.0 through 1.1.0;
- one IPv4 or IPv6 subnet with optional `rangeStart`, `rangeEnd`, `gateway`, and routes;
- requested addresses through `CNI_ARGS`;
- idempotent `ADD`, `CHECK`, and targeted `DEL` behavior;
- orphan cleanup through the 2016-07-29 metadata resources;
- an optional absolute `dataDir` and a log file below the managed platform log directory.

The plugin has no user interface or language catalog, so localization is not applicable to this repository.

## Build and test

Use Go 1.27.0:

```sh
go test ./...
go vet ./...
go mod verify
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o bin/host-local-cni-ipam .
bin/host-local-cni-ipam --version
```

The existing release covers the reviewed source and package gates. Linux race
tests, privileged CNI execution, metadata integration, upgrade, and rollback
tests remain required before claiming a fully supported host integration or
publishing a successor.

## Configuration

```json
{
  "cniVersion": "1.1.0",
  "name": "pasture-bridge",
  "ipam": {
    "type": "host-local-cni-ipam",
    "subnet": "192.0.2.0/24",
    "rangeStart": "192.0.2.20",
    "rangeEnd": "192.0.2.200",
    "gateway": "192.0.2.1",
    "metadataURL": "http://169.254.169.250/2016-07-29"
  }
}
```

`PLATFORM_METADATA_URL` overrides `metadataURL`, but both inputs are restricted to the reserved `169.254.169.250` and `169.254.169.251` platform addresses, the fixed `2016-07-29` API path, and standard HTTP or HTTPS ports. Metadata requests do not use ambient proxies or follow redirects.

`PLATFORM_CA_ROOT`, when set, must select the managed read-only mount at `/var/lib/pasturestack/etc/ssl/ca.crt`; arbitrary filesystem paths are rejected. File logging accepts the deployed `/var/log/pasturestack-cni.log` path or files below `/var/log/pasturestack`. If metadata is temporarily unavailable during `DEL`, the requested container lease is still released and only the optional orphan sweep is skipped.

## Security boundary

The plugin is intended to run as a privileged Linux CNI executable. Network names containing path separators are rejected, the data directory must be absolute, lease files are created with mode `0600`, metadata destinations are allowlisted at the final transport boundary, and operator-selected CA or log paths cannot escape their managed locations. See [SECURITY.md](SECURITY.md) for the remaining POC limitations.

## License and provenance

The repository is licensed under Apache-2.0. Existing source headers, authorship, and copyright attribution are retained. See [ORIGIN.md](ORIGIN.md), [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md), and [LICENSE](LICENSE).

# Compatibility

The POC keeps the historical single-subnet configuration and on-disk lease format. Existing lease files that contain only a container identifier remain readable.

The public executable, module path, IPAM type, environment variables, and log prefix use PastureStack-neutral names. The former environment-variable alias is intentionally not exposed in the new public contract.

Downstream deployment templates must switch to:

- executable and IPAM type: `host-local-cni-ipam`;
- metadata override: `PLATFORM_METADATA_URL`;
- optional CA bundle selector: `PLATFORM_CA_ROOT=/var/lib/pasturestack/etc/ssl/ca.crt`;
- metadata endpoint: `169.254.169.250` or `169.254.169.251` with the fixed `/2016-07-29` API path and a standard port;
- file logging: `/var/log/pasturestack-cni.log` or a file below `/var/log/pasturestack`.

The endpoint restrictions preserve the original link-local metadata design while closing arbitrary network and filesystem destinations introduced by configurable overrides. Operators using HTTPS keep the same trust behavior by mounting the PEM bundle at the managed CA path.

Compatibility with CNI 0.1.0 through 1.1.0 is implemented at the result boundary. A privileged Linux VM must still verify the existing data directory, CNI `ADD`/`CHECK`/`DEL`, metadata outage behavior, orphan cleanup, upgrade, and rollback.

# Security

## Reporting

Do not open a public issue containing credentials, private infrastructure data, lease contents, or exploit details. Use the organization security-reporting channel once it is published.

## POC limitations

- The plugin is Linux-only and normally runs with elevated CNI privileges.
- Metadata responses are capped at 8 MiB and requests have bounded timeouts.
- Metadata is restricted to the two reserved platform link-local addresses, the fixed API version path, and standard ports. Requests bypass ambient proxies, reject redirects, and are revalidated at the final HTTP transport boundary.
- A private metadata CA can only be read from `/var/lib/pasturestack/etc/ssl/ca.crt`. CNI log files can only use the deployed `/var/log/pasturestack-cni.log` path or remain below `/var/log/pasturestack`.
- Data-directory symlink races, filesystem rollback, concurrent privileged execution, and real metadata outage behavior still require isolated Linux VM testing.
- No release artifact is approved until dependency, vulnerability, license, current-tree brand, personal-data, race, and integration gates pass.

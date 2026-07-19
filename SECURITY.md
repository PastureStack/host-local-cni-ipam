# Security

## Reporting

Do not open a public issue containing credentials, private infrastructure data, lease contents, or exploit details. Use the organization security-reporting channel once it is published.

## POC limitations

- The plugin is Linux-only and normally runs with elevated CNI privileges.
- Metadata responses are capped at 8 MiB and requests have bounded timeouts.
- Cross-host metadata, data-directory symlink races, filesystem rollback, and concurrent privileged execution require VM testing.
- No release artifact is approved until dependency, vulnerability, license, current-tree brand, personal-data, race, and integration gates pass.

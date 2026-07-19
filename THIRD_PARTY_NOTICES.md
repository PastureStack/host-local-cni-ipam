# Third-party notices

The Linux build uses these fixed modules:

- `github.com/containernetworking/cni` v1.3.0 under Apache-2.0;
- `github.com/vishvananda/netns` v0.0.4 under Apache-2.0;
- `golang.org/x/sys` v0.23.0 under BSD-3-Clause.

Their exact license texts are retained under `LICENSES/` and checked by SHA-256 in `scripts/verify-licenses`.

Several allocator and disk-store files retain `Copyright 2015 CNI authors` or `Copyright 2016 CNI authors` headers. Those headers and the repository's historical authorship must not be removed or replaced.

The root `LICENSE` remains the unmodified license file inherited through the repository history.

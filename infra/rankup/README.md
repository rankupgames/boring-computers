# Isolated Unity-Unterm Worker

This profile runs one short-lived, headless Firecracker guest for auditing and
building the Unity-Unterm Rust sources. The control plane stays on loopback and
is reached through SSH. The guest receives no GitHub, Unity, cloud-model, or
WispKey vault credential.

## Trust boundaries

- `DEV-1` is the trusted coordinator and publisher. It owns WispKey and any
  repository write credential.
- `rug-boring-1` is the dedicated worker VM on a hypervisor separate from the
  coordinator. It runs
  `boringd`, the jailer, the egress firewall, and ephemeral Firecracker guests.
- The Firecracker guest clones public source, runs deterministic checks, and
  exports a binary Git patch plus checksums. It cannot push.
- The coordinator validates the patch paths, symlinks, checksums, and diff
  scope before applying it to a clean trusted checkout.

The worker VM should be provisioned with 4 vCPUs, 8 GiB RAM, 80 GiB local
storage, nested KVM, and no disks or caches on a stateful service dataset. Use a
host-only network with public-only NAT egress and reach it through the hypervisor
SSH jump host. Only the SSH alias is used by scripts; no address is committed.

## Fail-closed profile

`isolated-worker.env` enables the `isolated-worker` security profile. `boringd`
refuses to start unless all of these remain true:

- literal loopback bind and a non-empty control token loaded from a protected
  file;
- no bearer token in URLs, no CORS/proxy trust, and no preview routes;
- one machine, one machine per client, no persistent machines, no published
  templates, and a maximum TTL of 900 seconds;
- the reviewed unprivileged jailer UID/GID, cgroup CPU/PID limits, and guest
  egress networking enabled;
- no S3 persistence and no Anthropic or OpenRouter key.

The control daemon must retain the host privileges needed for KVM, TAP devices,
cgroups, and the jailer. Firecracker itself is launched chrooted under the
unprivileged jailer identity with per-VM CPU, memory, and PID caps. `boringd`
requires the guest-network service and re-validates its IPv4 egress policy and
IPv6 bridge block before accepting work; a partial network setup fails closed.

## Install

Install the checksum-pinned host runtime first. This installs the reviewed
Firecracker and jailer release, the reviewed CI kernel, Go, the rootfs build
dependencies, and the unprivileged jailer identity:

```bash
sudo ./infra/rankup/install-runtime.sh
```

Build and install the two Go binaries on the worker from a reviewed commit:

```bash
cd boringd
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /usr/local/bin/boringd .
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
  -o /usr/local/bin/wispkey-vsock-relay ./cmd/wispkey-vsock-relay
```

Install the control token without putting either it or the worker's sudo
credential in an argument, log, or repository file. The nested WispKey
processes inject them through a child-only environment variable and stdin:

```bash
wispkey exec --credential rug-boring-1-control-token \
  --env BORING_CONTROL_TOKEN -- \
  wispkey exec --credential rug-boring-1-sudo --stdin -- \
  ./infra/rankup/install-control-token.sh rug-boring-1
```

Then install and validate the profile on the worker:

```bash
sudo ./infra/rankup/install-worker-profile.sh
sudo /opt/boring/bin/build-unterm-builder-rootfs
sudo ./infra/rankup/install-worker-profile.sh --start
```

The installer fails unless both Firecracker and the jailer report the reviewed
`v1.16.1` release. A version override is permitted only for a separately
reviewed upgrade.

The rootfs build uses Ubuntu 24.04, pinned Rust and cargo-tool versions, and no
Node.js packages. It installs `rustfmt`, `clippy`, `cargo-audit`, `cargo-deny`,
clang, CMake, OpenSSL headers, Python for control-plane file transfer, and a
guest-side AF_VSOCK request helper. Override tool versions only in a reviewed
change; the defaults are deliberately not `latest`.

Reach the control API only through a tunnel:

```bash
ssh -o ExitOnForwardFailure=yes -N \
  -L 18080:127.0.0.1:8080 rug-boring-1
```

## Build-job contract

Create `unterm-builder` with `net:true` only for the public-source fetch and
dependency audit. The host firewall blocks loopback, link-local and metadata
ranges, private networks, the guest subnet, SMTP, and high-rate connection
scanning. The job must not receive a repository write token.

Unity-Unterm's current native library is explicitly macOS/Windows-only; its
Linux debugger backend is still a future path. On this Linux worker, run the
platform-independent repository gates and export any formatter changes:

```bash
git clone --filter=blob:none https://github.com/rankupgames/Unity-Unterm.git
cd Unity-Unterm/native
cargo fmt
cargo fmt --check
cargo metadata --locked --no-deps --format-version 1
cargo audit
cargo deny check
cd ..
git diff --binary --full-index > /tmp/unterm.patch
sha256sum /tmp/unterm.patch > /tmp/unterm.patch.sha256
```

The lifecycle proof separately creates a dependency-free Rust fixture inside
the same microVM and runs `cargo fmt --check`, `cargo check --locked`,
`cargo clippy --locked --all-targets -- -D warnings`, and `cargo test --locked`.
Do not claim a Linux build of the Unity native plugin until its shared-surface
and `unity-native-plugin` backends are implemented for Linux.

The trusted coordinator downloads only the patch and checksum, rejects absolute
paths, `..` traversal, symlinks, unexpected repositories, and paths outside the
approved source scope, and then reruns the checks before publishing.

## WispKey Firecracker verification

The Unity-Unterm build itself is secretless. Test the optional credential path
only with a disposable WispKey vault and two synthetic credential references:

```bash
sudo -v
BORING_URL=http://127.0.0.1:8080 ./infra/rankup/run-synthetic-wispkey-proof.sh
```

The wrapper creates a temporary WispKey vault, two synthetic credentials, and a
loopback-only target, then deletes the entire vault on exit. The harness proves
this sequence through guest AF_VSOCK:

1. an enrolled in-scope token reaches the synthetic target;
2. an out-of-scope token returns `403` and queues an access request;
3. host approval allows the retry;
4. instance revocation makes the original identity return `401`.

The harness suppresses guest TTY echo while injecting the one-time instance
identity, never prints token values, and revokes its enrolled instance. The
wrapper then removes the disposable vault, credentials, requests, and audit
state. The relay assigns its private `0600` Firecracker port socket to the
configured jailer UID/GID, so the unprivileged Firecracker process can connect
without making the credential channel available to other host users.
For a separated production topology, run WispKey on the coordinator with
identity required, carry its loopback listener through an SSH tunnel, and point
`wispkey-vsock-relay` only at the worker's loopback end. The relay rejects
non-loopback upstreams and logs no payloads.

## Verification

Run before publishing:

```bash
cd boringd
go test ./...
go vet ./...
go build ./...
cd ..
bash -n infra/rankup/*.sh
npm ci
npm run check
npm test
npm run lint
npm run build
```

The same gates run in GitHub Actions for pull requests and can be dispatched
manually against an exact reviewed branch when validating fork infrastructure.

The repository `.npmrc` enforces `min-release-age=7`; use npm 11.14.1 or newer
and do not replace `npm ci` with an unlocked install.

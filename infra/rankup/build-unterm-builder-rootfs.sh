#!/usr/bin/env bash
set -euo pipefail

boring_root="${BORING_ROOT:-/opt/boring}"
rootfs_dir="${boring_root}/rootfs"
image="${rootfs_dir}/unterm-builder.ext4"
image_size_mb="${IMAGE_SIZE_MB:-8192}"
rustup_version="${RUSTUP_VERSION:-1.28.2}"
rust_toolchain="${RUST_TOOLCHAIN:-1.91.1}"
cargo_audit_version="${CARGO_AUDIT_VERSION:-0.21.2}"
cargo_deny_version="${CARGO_DENY_VERSION:-0.18.4}"

platform_for_arch() {
	case "$1" in
		x86_64) printf '%s\t%s\t%s\n' "x86_64-unknown-linux-gnu" "http://archive.ubuntu.com/ubuntu" "http://security.ubuntu.com/ubuntu" ;;
		aarch64) printf '%s\t%s\t%s\n' "aarch64-unknown-linux-gnu" "http://ports.ubuntu.com/ubuntu-ports" "http://ports.ubuntu.com/ubuntu-ports" ;;
		*) echo "unsupported architecture" >&2; return 1 ;;
	esac
}

if [[ "${1:-}" == "--print-platform" ]]; then
	platform_for_arch "${2:-$(uname -m)}"
	exit
fi

if [[ "$(id -u)" -ne 0 ]]; then
	echo "run as root" >&2
	exit 1
fi

IFS=$'\t' read -r rust_target ubuntu_mirror ubuntu_security_mirror < <(platform_for_arch "$(uname -m)")

for command in debootstrap mkfs.ext4 mount mountpoint chroot curl sha256sum; do
	command -v "${command}" >/dev/null || { echo "missing required command: ${command}" >&2; exit 1; }
done

work="$(mktemp -d /tmp/boring-unterm-builder.XXXXXX)"
mount_dir="${work}/mnt"
mkdir -p "${mount_dir}"

cleanup() {
	result=$?
	umount -R "${mount_dir}/proc" 2>/dev/null || true
	umount -R "${mount_dir}/sys" 2>/dev/null || true
	umount -R "${mount_dir}/dev" 2>/dev/null || true
	if mountpoint -q "${mount_dir}"; then
		umount "${mount_dir}" 2>/dev/null || umount -l "${mount_dir}" 2>/dev/null || true
	fi
	rm -rf "${work}"
	return "${result}"
}
trap cleanup EXIT INT TERM

mkdir -p "${rootfs_dir}"
rm -f "${image}"
dd if=/dev/zero of="${image}" bs=1M count=0 seek="${image_size_mb}" status=none
mkfs.ext4 -q -F -O '^has_journal' "${image}"
mount -o loop "${image}" "${mount_dir}"

debootstrap --variant=minbase noble "${mount_dir}" "${ubuntu_mirror}"
cp -f /etc/resolv.conf "${mount_dir}/etc/resolv.conf"
cat > "${mount_dir}/etc/apt/sources.list" <<EOF
deb ${ubuntu_mirror} noble main universe
deb ${ubuntu_mirror} noble-updates main universe
deb ${ubuntu_security_mirror} noble-security main universe
EOF
mount -t proc proc "${mount_dir}/proc"
mount -t sysfs sysfs "${mount_dir}/sys"
mount --bind /dev "${mount_dir}/dev"

chroot "${mount_dir}" /bin/bash -euo pipefail <<'CHROOT'
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
	build-essential ca-certificates clang cmake curl git jq libssl-dev pkg-config \
	python3 unzip xz-utils
passwd -d root
apt-get clean
rm -rf /var/lib/apt/lists/*
CHROOT

# The build-time chroot can use the host resolver, but the booted microVM has no
# systemd-resolved stub at 127.0.0.53. Route guest DNS through the isolated
# bridge's dnsmasq listener instead.
rm -f "${mount_dir}/etc/resolv.conf"
cat > "${mount_dir}/etc/resolv.conf" <<'RESOLV'
nameserver 10.200.0.1
options timeout:2 attempts:3
RESOLV

rustup_url="https://static.rust-lang.org/rustup/archive/${rustup_version}/${rust_target}/rustup-init"
curl -fsSL "${rustup_url}" -o "${mount_dir}/tmp/rustup-init"
rustup_checksum_record="$(curl -fsSL "${rustup_url}.sha256")"
rustup_sha256="${rustup_checksum_record%% *}"
rustup_checksum_name="${rustup_checksum_record#* }"
[[ "${rustup_sha256}" =~ ^[0-9a-f]{64}$ && "${rustup_checksum_name}" == "*./rustup-init" ]] || {
	echo "invalid rustup checksum response" >&2
	exit 1
}
printf '%s  rustup-init\n' "${rustup_sha256}" > "${mount_dir}/tmp/rustup-init.sha256"
(
	cd "${mount_dir}/tmp"
	sha256sum -c rustup-init.sha256
)
chmod 0755 "${mount_dir}/tmp/rustup-init"

chroot "${mount_dir}" /usr/bin/env \
	RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo \
	/tmp/rustup-init -y --profile minimal --default-toolchain "${rust_toolchain}"
chroot "${mount_dir}" /usr/bin/env \
	PATH=/opt/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
	RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo \
	/opt/cargo/bin/rustup component add --toolchain "${rust_toolchain}" rustfmt clippy
chroot "${mount_dir}" /usr/bin/env \
	PATH=/opt/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
	RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo \
	/opt/cargo/bin/cargo install --locked --version "${cargo_audit_version}" cargo-audit
chroot "${mount_dir}" /usr/bin/env \
	PATH=/opt/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
	RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo \
	/opt/cargo/bin/cargo install --locked --version "${cargo_deny_version}" cargo-deny

cat > "${mount_dir}/etc/profile.d/rust.sh" <<'PROFILE'
export RUSTUP_HOME=/opt/rustup
export CARGO_HOME=/opt/cargo
export PATH=/opt/cargo/bin:$PATH
PROFILE

install -d -m0755 \
	"${mount_dir}/etc/systemd/system/serial-getty@ttyS0.service.d" \
	"${mount_dir}/etc/systemd/system/multi-user.target.wants" \
	"${mount_dir}/etc/systemd/system/getty.target.wants"
cat > "${mount_dir}/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf" <<'GETTY'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I 115200,38400,9600 vt220
GETTY

cat > "${mount_dir}/etc/systemd/system/boring-ready.service" <<'READY'
[Unit]
Description=Signal Firecracker guest readiness
After=systemd-remount-fs.service

[Service]
Type=oneshot
ExecStart=/bin/sh -c 'echo BORING_READY > /dev/ttyS0'

[Install]
WantedBy=multi-user.target
READY
ln -s ../boring-ready.service "${mount_dir}/etc/systemd/system/multi-user.target.wants/boring-ready.service"
ln -s /lib/systemd/system/serial-getty@.service "${mount_dir}/etc/systemd/system/getty.target.wants/serial-getty@ttyS0.service"

cat > "${mount_dir}/usr/local/bin/wispkey-vsock-request" <<'PYTHON'
#!/usr/bin/env python3
import json
import os
import socket
import sys


def required(name):
    value = os.environ.get(name, "")
    if not value or "\r" in value or "\n" in value:
        raise SystemExit(f"missing or invalid {name}")
    return value


def decode_chunked(body):
    decoded = bytearray()
    while body:
        line, separator, body = body.partition(b"\r\n")
        if not separator:
            raise SystemExit("invalid chunked response")
        size = int(line.split(b";", 1)[0], 16)
        if size == 0:
            return bytes(decoded)
        decoded.extend(body[:size])
        body = body[size + 2:]
    raise SystemExit("truncated chunked response")


instance_id = required("WISPKEY_INSTANCE_ID")
instance_secret = required("WISPKEY_INSTANCE_SECRET")
wisp_token = required("WISPKEY_TOKEN")
target_url = required("WISPKEY_TARGET_URL")
port = int(os.environ.get("WISPKEY_VSOCK_PORT", "7700"))

request = (
    "GET / HTTP/1.1\r\n"
    "Host: wispkey.local\r\n"
    f"X-Target-Url: {target_url}\r\n"
    f"Authorization: Bearer {wisp_token}\r\n"
    f"x-wispkey-instance-id: {instance_id}\r\n"
    f"x-wispkey-instance-secret: {instance_secret}\r\n"
    "Connection: close\r\n\r\n"
).encode()

connection = socket.socket(socket.AF_VSOCK, socket.SOCK_STREAM)
connection.settimeout(20)
connection.connect((2, port))
connection.sendall(request)
response = bytearray()
while True:
    block = connection.recv(65536)
    if not block:
        break
    response.extend(block)
connection.close()

headers, separator, body = bytes(response).partition(b"\r\n\r\n")
if not separator:
    raise SystemExit("invalid HTTP response")
status = int(headers.split(b" ", 2)[1])
if b"transfer-encoding: chunked" in headers.lower():
    body = decode_chunked(body)

result = {"status": status}
if status == 403 and os.environ.get("WISPKEY_SHOW_DENIAL") == "1":
    denial = json.loads(body)
    result["error"] = denial.get("error")
    result["access_request"] = denial.get("access_request")
json.dump(result, sys.stdout, separators=(",", ":"))
sys.stdout.write("\n")
PYTHON
chmod 0755 "${mount_dir}/usr/local/bin/wispkey-vsock-request"

sync
umount "${mount_dir}/proc"
umount "${mount_dir}/sys"
umount "${mount_dir}/dev"
umount "${mount_dir}"
e2fsck -fy "${image}" >/dev/null
echo "built ${image}"

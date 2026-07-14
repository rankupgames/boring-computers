#!/usr/bin/env bash
set -euo pipefail

firecracker_version="v1.16.1"
go_version="1.25.0"
kernel_version="6.1.102"

case "$(uname -m)" in
	x86_64)
		firecracker_sha256="382a02a869e4d6d5cb14c40577f9545e8458021ea8b0b2d3fc10ec14d9c242e6"
		go_arch="amd64"
		go_sha256="2852af0cb20a13139b3448992e69b868e50ed0f8a1e5940ee1de9e19a123b613"
		kernel_sha256="49ba99a5299444ac59dda2efc3569cc2d58a5d72ea6475a6bfc37aa0bf322e54"
		platform="x86_64"
		;;
	aarch64)
		firecracker_sha256="8d0e69f6d6f9a1724551f607f18504052c16c1828ee3d4d7b6e6c73380871e0e"
		go_arch="arm64"
		go_sha256="05de75d6994a2783699815ee553bd5a9327d8b79991de36e38b66862782f54ae"
		kernel_sha256="bb1f50912d63a8ca5e92d488984875e1177eb9283050ffa592a8cb455cada52d"
		platform="aarch64"
		;;
	*)
		echo "unsupported architecture" >&2
		exit 1
		;;
esac

if [[ "$(id -u)" -ne 0 ]]; then
	echo "run as root" >&2
	exit 1
fi
[[ -c /dev/kvm && -r /dev/kvm && -w /dev/kvm ]] || {
	echo "/dev/kvm is unavailable" >&2
	exit 1
}

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
	build-essential ca-certificates cpio curl debootstrap dnsmasq e2fsprogs file git \
	iproute2 iptables jq shellcheck tar util-linux

work="$(mktemp -d /tmp/boring-runtime.XXXXXX)"
cleanup() {
	result=$?
	rm -rf "${work}"
	return "${result}"
}
trap cleanup EXIT INT TERM

firecracker_archive="firecracker-${firecracker_version}-${platform}.tgz"
curl -fL --retry 3 \
	"https://github.com/firecracker-microvm/firecracker/releases/download/${firecracker_version}/${firecracker_archive}" \
	-o "${work}/${firecracker_archive}"
printf '%s  %s\n' "${firecracker_sha256}" "${work}/${firecracker_archive}" | sha256sum -c -
tar -xzf "${work}/${firecracker_archive}" -C "${work}"
release_dir="${work}/release-${firecracker_version}-${platform}"
for binary in firecracker jailer; do
	source_path="${release_dir}/${binary}-${firecracker_version}-${platform}"
	[[ -x "${source_path}" ]] || { echo "missing ${binary} in reviewed release" >&2; exit 1; }
	install -d -m0755 /opt/boring/bin
	install -m0755 "${source_path}" "/opt/boring/bin/${binary}"
	"/opt/boring/bin/${binary}" --version | grep -Fq "${firecracker_version}"
done

kernel_url="https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.10/${platform}/vmlinux-${kernel_version}"
curl -fL --retry 3 "${kernel_url}" -o "${work}/vmlinux"
printf '%s  %s\n' "${kernel_sha256}" "${work}/vmlinux" | sha256sum -c -
install -d -m0755 /opt/boring/kernel
install -m0644 "${work}/vmlinux" /opt/boring/kernel/vmlinux

go_archive="go${go_version}.linux-${go_arch}.tar.gz"
curl -fL --retry 3 "https://go.dev/dl/${go_archive}" -o "${work}/${go_archive}"
printf '%s  %s\n' "${go_sha256}" "${work}/${go_archive}" | sha256sum -c -
rm -rf /usr/local/go
tar -C /usr/local -xzf "${work}/${go_archive}"
ln -sfn /usr/local/go/bin/go /usr/local/bin/go
ln -sfn /usr/local/go/bin/gofmt /usr/local/bin/gofmt
[[ "$(go version)" == go\ version\ go${go_version}* ]]

if getent group 30000 >/dev/null; then
	[[ "$(getent group 30000 | cut -d: -f1)" == "boringjail" ]] || {
		echo "gid 30000 is already assigned" >&2
		exit 1
	}
else
	groupadd --gid 30000 boringjail
fi
if getent passwd 30000 >/dev/null; then
	[[ "$(getent passwd 30000 | cut -d: -f1)" == "boringjail" ]] || {
		echo "uid 30000 is already assigned" >&2
		exit 1
	}
else
	useradd --uid 30000 --gid 30000 --no-create-home --shell /usr/sbin/nologin boringjail
fi
install -d -m0755 /srv/jailer

echo "installed reviewed Firecracker ${firecracker_version}, kernel ${kernel_version}, and Go ${go_version}"

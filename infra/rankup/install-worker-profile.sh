#!/usr/bin/env bash
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
	echo "run as root" >&2
	exit 1
fi

expected_firecracker_version="${FIRECRACKER_VERSION:-v1.16.1}"
for binary in firecracker jailer; do
	path="/opt/boring/bin/${binary}"
	[[ -x "${path}" ]] || { echo "missing required binary: ${path}" >&2; exit 1; }
	"${path}" --version | grep -Fq "${expected_firecracker_version}" || {
		echo "${binary} does not match pinned Firecracker ${expected_firecracker_version}" >&2
		exit 1
	}
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
install -d -m0755 /etc/boring /opt/boring/bin
install -m0644 "${script_dir}/isolated-worker.env" /etc/boring/isolated-worker.env
install -m0644 "${script_dir}/boringd-isolated-worker.service" /etc/systemd/system/boringd.service
install -m0755 "${script_dir}/build-unterm-builder-rootfs.sh" /opt/boring/bin/build-unterm-builder-rootfs

systemctl daemon-reload
/usr/local/bin/boringd -check-config

if [[ "${1:-}" == "--start" ]]; then
	systemctl enable --now boringd.service
fi

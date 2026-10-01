#!/usr/bin/env bash
# Explicit EL9 administrator setup. Does not change networking or VNC sessions.
set -euo pipefail
if [[ $EUID != 0 || $# != 1 ]]; then
    echo 'Usage: sudo bash install-linux-shared-desktop.sh DESKTOP_USER' >&2
    exit 2
fi
owner=$1
bundle=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
archive="$bundle/pipewire-module-xrdp-v0.2.tar"
expected=238af37642b197caaecc28f74a1c8b176f5f7f728810c603c43587b52e25dac9
[[ $(sha256sum "$archive" | cut -d ' ' -f 1) == "$expected" ]] || {
    echo 'Pinned upstream source checksum mismatch' >&2; exit 1;
}
# Check mapping before installing anything. No credentials are read or copied.
python3 "$bundle/linux-shared-desktop.py" plan --user "$owner"
dnf install -y gcc make autoconf automake libtool pkgconf-pkg-config pipewire-devel
build=$(mktemp -d /var/tmp/pfremote-audio-build.XXXXXXXX)
chmod 700 "$build"
trap 'rm -rf -- "$build"' EXIT
tar -xf "$archive" -C "$build"
cd "$build/pipewire-module-xrdp"
./bootstrap
./configure
make -j2
module_dir=$(pkg-config --variable=moduledir libpipewire-0.3)
case "$module_dir" in /usr/lib/pipewire-0.3|/usr/lib64/pipewire-0.3) ;; *)
    echo 'Unexpected PipeWire module directory' >&2; exit 1;; esac
module="$module_dir/libpipewire-module-xrdp.so"
if [[ -e "$module" ]]; then
    echo 'An existing audio module needs review; it has not been overwritten.' >&2
    exit 1
fi
# Install only the module, not upstream global desktop autostart/default changes.
install -m 755 src/.libs/libpipewire-module-xrdp.so "$module"
if command -v restorecon >/dev/null; then restorecon "$module"; fi
if ! python3 "$bundle/linux-shared-desktop.py" install --user "$owner"; then
    rm -f -- "$module"
    echo 'Setup failed. Original xrdp configuration was retained or restored.' >&2
    exit 1
fi
echo 'Server configuration installed. RDP/VNC continuity and two-way sound still need live verification.'

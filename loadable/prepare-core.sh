#!/bin/sh
# prepare-core.sh ROOT GO_VERSION
#
# Rewrites ROOT/go.mod, a COPY of the shared Replay Lab core's module, so the
# loadable build selects exactly the target gateway's module versions.
#
# The core's own go.mod requires newer versions of modules it shares with the
# gateway (Bifrost core, sonic, x/sys, ...). Minimal version selection would pick
# those, and a `replace` back down doesn't help: with -trimpath, a replaced
# module's file paths still carry the required version, which changes the
# package's fingerprint, and the gateway rejects the plugin with "plugin was
# built with a different version of package ...". So the copy declares no
# requirements at all; loadable/go.mod lists what the core imports, and the
# target release's requirements decide every shared version.
#
# It also sets the go directive to the gateway's Go version, since the core
# asks for a newer Go than the gateway's toolchain.
set -eu
root=$1
go_version=$2
module=$(awk '$1 == "module" { print $2; exit }' "$root/go.mod")
printf 'module %s\n\ngo %s\n' "$module" "$go_version" > "$root/go.mod"
rm -f "$root/go.sum"

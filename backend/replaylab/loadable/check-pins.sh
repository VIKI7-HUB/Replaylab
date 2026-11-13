#!/bin/sh
# check-pins.sh GATEWAY_SRC_DIR LOADABLE_DIR [BUILD_TAGS]
#
# A Go plugin only loads if every package it shares with the gateway was built
# from the same module version. For each module that supplies a package linked
# into the plugin, compares the version selected for the plugin with the one
# the gateway's build list selects, and fails on any difference.
#
# Fix a failure by changing loadable/go.mod's requirements so minimal version
# selection lands on the gateway's version (usually: an older version of the
# Replay-Lab-only dependency that pulled the newer one in). Don't use `replace`
# for this: under -trimpath a replaced module's paths still carry the required
# version, and the plugin's package fingerprints won't match.
set -eu
gw_dir=$1
pl_dir=$2
tags=${3:-replaylab_core_v1_7}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

(cd "$gw_dir" && go list -m -f '{{if not .Main}}{{.Path}} {{.Version}}{{end}}' all) | grep . > "$tmp/gateway"
(cd "$pl_dir" && GOOS=linux go list -deps -tags "$tags" \
	-f '{{with .Module}}{{if not .Main}}{{.Path}} {{if .Replace}}{{.Replace.Path}}@{{if .Replace.Version}}{{.Replace.Version}}{{else}}local{{end}}{{else}}{{.Version}}{{end}}{{end}}{{end}}' .) \
	| grep . | sort -u > "$tmp/plugin"

if awk '$2 ~ /@/ && $2 !~ /@local$/' "$tmp/plugin" | grep -q .; then
	echo "check-pins: these modules are replaced; -trimpath would record the wrong version:" >&2
	awk '$2 ~ /@/ && $2 !~ /@local$/ { print "  " $0 }' "$tmp/plugin" >&2
	exit 1
fi

if ! awk 'NR == FNR { v[$1] = $2; next }
	($1 in v) && v[$1] != $2 { printf "  %s: gateway %s, plugin %s\n", $1, v[$1], $2; bad = 1 }
	END { exit bad }' "$tmp/gateway" "$tmp/plugin" > "$tmp/diff"; then
	echo "check-pins: plugin and gateway disagree on module versions:" >&2
	cat "$tmp/diff" >&2
	exit 1
fi
shared=$(awk 'NR == FNR { v[$1] = 1; next } ($1 in v)' "$tmp/gateway" "$tmp/plugin" | wc -l)
echo "check-pins: $(wc -l < "$tmp/plugin" | tr -d ' ') modules linked into the plugin, $shared shared with the gateway, all at the gateway's version"

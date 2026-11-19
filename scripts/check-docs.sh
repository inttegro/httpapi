#!/usr/bin/env bash

set -euo pipefail

repository_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
profile_projection="$(mktemp)"
v0_2_projection="$(mktemp)"
trap 'rm -f "$profile_projection" "$v0_2_projection"' EXIT

tail -n +7 "$repository_dir/docs/httpapi/specification/v0.2.md" >"$v0_2_projection"
cmp "$repository_dir/spec/v0.2.md" "$v0_2_projection"
cmp "$repository_dir/spec/v0.1.md" "$repository_dir/docs/httpapi/specification/v0.1.md"

sed 's#../../v0.2\.md#/httpapi/specification/v0.2#g' \
	"$repository_dir/spec/profiles/openapi-3.1/v0.1.md" >"$profile_projection"
cmp "$profile_projection" "$repository_dir/docs/httpapi/profiles/openapi-3.1-v0.1.md"

echo "documentation mirrors match their normative sources"

#!/usr/bin/env bash

set -euo pipefail

GITHUB_TOKEN='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'

get_last_page() {
	local url="$1"
	curl -sSf -I -H "Authorization: Bearer $GITHUB_TOKEN" \
		"$url" |
		grep -E '^link: ' |
		sed -e 's/^link:.*page=//g' -e 's/>.*$//g' || echo "1"
}

generate() {
	local url="$1"
	local file="$2"
	last_page="$(get_last_page "$url")"
	tmp="$(mktemp -d)"

	for i in $(seq 1 "$last_page"); do
		echo "page: $i"
		curl -H "Authorization: Bearer $GITHUB_TOKEN" -sSf "$url?page=$i" | jq 'map({tag_name: .tag_name})' >"$tmp/$i.json"
	done

	if test "$last_page" -eq "1"; then
		cp -f "$tmp"/1.json "$file"
	else
		jq --compact-output -s 'add' "$tmp"/*.json >"$file"
	fi
	du -hs "$file"
}

# www/docs/static/releases.json
generate "https://api.github.com/repos/foundry-rs/foundry/releases" "releases.json"
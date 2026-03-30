
#!/usr/bin/env bash
set -euo pipefail

# Script to download Cloudflare IPv4 and IPv6 lists and write combined unique CIDRs
# into ../src/mod/access/default_trusted_proxies.csv (relative to script dir).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/../src/mod/access/default_trusted_proxies.csv"
mkdir -p "$(dirname "$TARGET")"

TMP="$(mktemp)"
TMP2="$(mktemp)"
cleanup() {
	rm -f "$TMP" "$TMP2"
}
trap cleanup EXIT

# Cloudflare provides plain text lists at these URLs
urls=(
	"https://www.cloudflare.com/ips-v4"
	"https://www.cloudflare.com/ips-v6"
)

for url in "${urls[@]}"; do
	if ! curl -fsSL "$url" -o "$TMP"; then
		echo "Failed to download $url" >&2
		exit 1
	fi
	# The response is plain text with one CIDR per line
	# Filter out empty lines and append to temp file
	grep -v '^[[:space:]]*$' "$TMP" >> "$TMP2" || true
done

# sort unique and write to target (one CIDR per line)
sort -u "$TMP2" > "$TARGET"
chmod 644 "$TARGET"

echo "Wrote $(wc -l < "$TARGET") entries to $TARGET"

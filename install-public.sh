#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "این نصب‌کننده باید با root اجرا شود." >&2
  exit 1
fi

for command_name in curl tar; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    apt-get update
    apt-get install -y curl tar
  fi
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "دریافت نسخه main از GitHub..."
curl -fsSL "https://api.github.com/repos/hatinati2/rabin-assistant/tarball/main" -o "$tmp/source.tar.gz"
mkdir "$tmp/source"
tar -xzf "$tmp/source.tar.gz" --strip-components=1 -C "$tmp/source"

echo "شروع نصب Rabin Assistant..."
bash "$tmp/source/deploy/install.sh"

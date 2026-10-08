#!/usr/bin/env bash
# Проверка артефактов GitHub Release: checksums + cosign + slsa-verifier.
#
# Требования: curl, cosign, slsa-verifier; sha256sum или shasum.
#   go install github.com/sigstore/cosign/v2/cmd/cosign@latest
#   go install github.com/slsa-framework/slsa-verifier/v2/cli/slsa-verifier@latest
#
# Использование:
#   ./scripts/verify-release.sh v0.1.0
#   make verify-release TAG=v0.1.0
#   REPO=AESalnikov/depscout COSIGN_IDENTITY=... ./scripts/verify-release.sh v0.1.0
set -euo pipefail

TAG="${1:-}"
if [[ -z "$TAG" ]]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 2
fi

REPO="${REPO:-AESalnikov/depscout}"
SOURCE_URI="github.com/${REPO}"
BASE="https://github.com/${REPO}/releases/download/${TAG}"
WORKDIR="${TMPDIR:-/tmp}/depscout-verify-${TAG}"
mkdir -p "$WORKDIR"
cd "$WORKDIR"
echo "workdir: $WORKDIR"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing tool: $1" >&2
    exit 1
  }
}
need curl
need cosign
need slsa-verifier

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

echo "==> download checksums + signature bundle"
curl -fsSL -O "${BASE}/checksums.txt"
curl -fsSL -O "${BASE}/checksums.txt.sigstore.json"

echo "==> verify cosign signature on checksums.txt"
COSIGN_IDENTITY="${COSIGN_IDENTITY:-https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${TAG}}"
cosign verify-blob \
  --certificate-identity "${COSIGN_IDENTITY}" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

echo "==> download + check archives from checksums.txt"
while read -r expect name; do
  [[ -z "${name:-}" ]] && continue
  case "$name" in
    *.tar.gz|*.zip) ;;
    *) continue ;;
  esac
  echo "  get $name"
  curl -fsSL -O "${BASE}/${name}"
  got="$(file_sha256 "$name")"
  if [[ "$got" != "$expect" ]]; then
    echo "checksum mismatch for $name: got $got want $expect" >&2
    exit 1
  fi
done < checksums.txt

echo "==> download SLSA provenance"
PROV=""
if [[ -n "${PROVENANCE:-}" ]]; then
  curl -fsSL -o provenance.intoto.jsonl "${BASE}/${PROVENANCE}"
  PROV=provenance.intoto.jsonl
else
  for candidate in \
    "multiple.intoto.jsonl" \
    "depscout.intoto.jsonl" \
    "provenance.intoto.jsonl"
  do
    if curl -fsSL -o "$candidate" "${BASE}/${candidate}"; then
      PROV="$candidate"
      break
    fi
  done
fi
if [[ -z "$PROV" ]]; then
  echo "provenance not found; set PROVENANCE=<asset-name> from the GitHub release page" >&2
  exit 1
fi
echo "  using $PROV"

echo "==> slsa-verifier"
while read -r _ name; do
  [[ -z "${name:-}" ]] && continue
  case "$name" in
    *.tar.gz|*.zip) ;;
    *) continue ;;
  esac
  [[ -f "$name" ]] || continue
  echo "  verify $name"
  slsa-verifier verify-artifact "$name" \
    --provenance-path "$PROV" \
    --source-uri "$SOURCE_URI" \
    --source-tag "$TAG"
done < checksums.txt

echo "OK: ${TAG} artifacts verified"

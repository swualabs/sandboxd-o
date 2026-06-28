#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/dist"
OUT_NAME="${1:-sbx-build-artifacts.zip}"
OUT_PATH="${OUT_DIR}/${OUT_NAME}"

REQUIRED_FILES=(
  "scripts/install.sh"
)

cd "${ROOT_DIR}"

if ! command -v zip >/dev/null 2>&1; then
  echo "error: zip command not found"
  exit 1
fi

make build

if [ ! -d "build" ]; then
  echo "error: build directory not found"
  exit 1
fi

if [ ! -d "configs" ]; then
  echo "error: configs directory not found"
  exit 1
fi

for file in "${REQUIRED_FILES[@]}"; do
  if [ ! -f "${file}" ]; then
    echo "error: required file not found: ${file}"
    exit 1
  fi
done

mkdir -p "${OUT_DIR}"

STAGING_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${STAGING_DIR}"
}
trap cleanup EXIT

mkdir -p "${STAGING_DIR}/build"
mkdir -p "${STAGING_DIR}/scripts"
mkdir -p "${STAGING_DIR}/configs"

cp -a build/. "${STAGING_DIR}/build/"
cp -a scripts/install.sh "${STAGING_DIR}/scripts/"
cp -a configs/. "${STAGING_DIR}/configs/"

find "${STAGING_DIR}" -name ".DS_Store" -delete

rm -f "${OUT_PATH}"

(
  cd "${STAGING_DIR}"
  zip -r "${OUT_PATH}" build scripts configs >/dev/null
)

echo "done: ${OUT_PATH}"

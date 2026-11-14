#!/bin/bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE_PATH="devops/docker/Dockerfile.deb"
IMAGE_NAME="moderator-launcher-deb-builder"
PACKAGE_ARCH="amd64"
PACKAGE_VERSION="${PACKAGE_VERSION:-}"

case "$PACKAGE_VERSION" in
    ''|[!0-9]*|*[!0-9A-Za-z.+:~-]*)
        echo "PACKAGE_VERSION is required and must be a Debian version starting with a digit" >&2
        echo "Usage: PACKAGE_VERSION=1.2.3 ./devops/build-deb.sh" >&2
        exit 1
        ;;
esac

ARTIFACT_NAME="moderator-launcher_${PACKAGE_VERSION}_${PACKAGE_ARCH}.deb"
CONTAINER_ID=""
cleanup() {
    if [[ -n "$CONTAINER_ID" ]]; then
        docker rm "$CONTAINER_ID" >/dev/null
    fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

echo "Project root: ${PROJECT_ROOT}"
echo "Package version: ${PACKAGE_VERSION}"
make -C "$PROJECT_ROOT" build-linux-astra

echo "Building Debian package image..."
docker build \
    --platform "linux/${PACKAGE_ARCH}" \
    --target builder \
    --build-arg "PACKAGE_VERSION=${PACKAGE_VERSION}" \
    --build-arg "PACKAGE_ARCH=${PACKAGE_ARCH}" \
    -t "$IMAGE_NAME" \
    -f "${PROJECT_ROOT}/${DOCKERFILE_PATH}" \
    "$PROJECT_ROOT"

echo "Creating container and extracting artifact..."
CONTAINER_ID=$(docker create --platform "linux/${PACKAGE_ARCH}" "$IMAGE_NAME" sh -c "true")
mkdir -p "${PROJECT_ROOT}/dist"
docker cp "${CONTAINER_ID}:/tmp/${ARTIFACT_NAME}" "${PROJECT_ROOT}/dist/${ARTIFACT_NAME}"
echo "Deb package saved as: ${PROJECT_ROOT}/dist/${ARTIFACT_NAME}"

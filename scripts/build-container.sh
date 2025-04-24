#!/bin/bash

# abbreviated git tag
export TAG
export DATE
export EXIT_CODE
export FULL_TAG
export SHORT_SHA

TAG="v0.0.0"
DATE=$(date +%Y-%m-%d_%H-%M-%S)
FULL_TAG="${TAG}-${DATE}"

SHORT_SHA="$(git rev-parse --short HEAD)"
# containerize that shit
DOCKER_BUILDKIT=1 docker build -t esacteksab/gh-actlock-test:"${FULL_TAG}" .

echo "${FULL_TAG}" > .current-tag

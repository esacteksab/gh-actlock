#!/bin/bash

# abbreviated git tag
export TAG
TAG="$(git describe --tags --abbrev=0)"
# containerize that shit
docker build -t repository/image:"${TAG}" .

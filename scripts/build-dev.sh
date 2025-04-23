#!/bin/bash
#
# The release script for gh-tf-pr to build a Go binary for local development

#err() {
#  echo "[$(date +'%Y-%m-%dT%H:%M:%S%z')]: $*" >&2
#}

export SHORT_SHA
export DATE
export VERSION

SHORT_SHA="$(git rev-parse --short=10 HEAD)"

DATE="$(date +%Y-%m-%d-%H:%M:%S)"

# Determines if there are local/unstaged changes
# If there are, we append a string '.dirty' to ${VERSION}
is_dirty() {
  local DIRTY

  git status > /dev/null 2>&1
  DIRTY=$(git diff-index --quiet HEAD || echo ".dirty")

  echo "$DIRTY"
  unset DIRTY
}

# Checks to see if a Git tag exists. If one doesn't, we don't want the error message so,  2> /dev/null
git_tag() {
  git describe --tags HEAD 2> /dev/null
}

# If a git tag doesn't exist, we get a ${VERSION} like
# gh-tf-pr --version
# tf-pr version 2025-02-22-10:30:29-ce6c09843f.dirty
#
# If a tag exists, we get a ${VERSION} like
# gh-tf-pr --version
# tf-pr version v0.0.1-alpha1.2025-02-22-10:31:34-ce6c09843f.dirty
version() {
  if [[ ! $(git_tag) ]]; then
    echo "${DATE}-${SHORT_SHA}$(is_dirty)"
  else
    echo "$(git_tag).${DATE}-${SHORT_SHA}$(is_dirty)"
  fi
}

VERSION="$(version)"

# build local binary
go build -v -ldflags "-X github.com/owner/repo/cmd.Version=${VERSION} -X github.com/owner/repo/cmd.BuiltBy=yoMomma!"

## Motivation

I've started dipping my toes back into Go and I found that there is a common set of tools I use. This is an attempt to capture that to reduce duplication and toil. This is not meant to be a one-size-fits-all or an all-in-one solution, rather, it's a boilerplate. While this is focused towards Go, you could remove the Go stuff and still have a good foundation. I just didn't want to spiral or run down the rabbit hole of what it might look like to have a base template, then language-specific templates.

## Tools

In no particular order, what exists here is defined below.

<!-- keep-sorted start case=no -->

- [check-jsonschema](https://check-jsonschema.readthedocs.io/en/latest/)
- [Dependabot](https://github.com/dependabot)
- [EditorConfig](https://editorconfig.org/)
- [golangci-lint](https://golangci-lint.run/)
- [GoReleaser](https://goreleaser.com/)
- [keep-sorted](https://github.com/google/keep-sorted)
- [Makefile](https://www.gnu.org/software/make/manual/make.html#Introduction)
- [markdownlint-cli2](https://github.com/DavidAnson/markdownlint-cli2)
- [Mdformat](https://mdformat.readthedocs.io/en/stable/)
- [pre-commit](https://pre-commit.com/)
- [Prettier](https://prettier.io/)
- [shellcheck-py](https://github.com/shellcheck-py/shellcheck-py)
- [typos](https://github.com/crate-ci/typos/)

<!-- keep-sorted end -->

This is how **_I_** set up a Go project hosted on GitHub. I'm a novice. It's likely "wrong". Like everything I do, it's founded in an attempt to learn and to continue to grow, so I'm open to your ideas and suggestions. Pull requests welcome, but understand this reflects how _my_ brain works and while your PR may make sense, it may not match my mental model or workflow. If you like what's here, and want to use it but wish something were different and I don't accept the PR, don't take it personally, fork this repo and create your own template. But please don't let this message discourage you from opening a PR or creating an Issue to start a discussion, I don't know what I don't know, if you see something, say something, please.

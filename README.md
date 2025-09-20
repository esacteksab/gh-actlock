# gh-actlock

`gh-actlock` is a GitHub CLI extension that improves the security of your GitHub Actions workflows by automatically pinning action references to specific commit SHAs.

## Features

- Pins GitHub Actions and shared workflows to full commit SHAs
- Handles all formats: tags, branches, and already-pinned SHAs
- Preserves original references as in-line comments
- Implements local HTTP caching to reduce API calls
- Preserves file formatting, indentation, and syntax
- Updates pinned SHAs to latest[^1] versions with `-u/--update` flag

## Why Pin GitHub Actions?

GitHub Actions are typically referenced using a tag or branch name:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-node@main
```

This approach has security implications:

- Tags can be moved to point to different commits
- Branches can be updated with new, potentially malicious code
- Supply chain attacks become possible if action repositories are compromised

By pinning actions to specific commit SHAs, you make your workflows more secure:

```yaml
steps:
  - uses: actions/checkout@a81bbbf8298c0fa03ea29cdc473d45769f953675  # v4
  - uses: actions/setup-node@8f152de45cc393bb48ce5d89d36b731f54556e65  # main
```

## Installation

### Prerequisites

- [GitHub CLI](https://cli.github.com/) (`gh`) installed

### Install as a GitHub CLI extension

```bash
gh ext install esacteksab/gh-actlock
```

## Usage

> [!NOTE]
> `gh-actlock` is designed to be run in the root directory of your Git repository. It expects a `.github/` or `.github/workflows/` directory containing your action or workflow files.

### Commands

- `gh actlock`: Default command to pin actions and shared workflows to the full commit SHA of the current ref.
- `gh actlock -u` or `gh actlock --update`: Update existing pinned SHAs to latest[^1] versions.
- `gh actlock clear -f` or `gh actlock clear --force`: Clear the local cache.

Navigate to your repository's root directory and run:

```bash
gh actlock
```

The extension will:

1. Find all action files in `.github/` or workflow files in `.github/workflows/`.
1. Analyze each file and identify any action or shared workflow references.
1. Resolve non-SHA references (tags, branches) to their corresponding full commit SHAs.
1. Update each action or workflow file with the full commit SHA of the existing reference, preserving the original reference as an inline comment.

> [!IMPORTANT]
> Make sure you run the command from your repository's root directory where the `.github/` directory is located.

### Updating Pinned Actions and Shared Workflows

To update actions and shared workflows that are already pinned to SHAs to their latest[^1] versions, use the `-u` or `--update` flag:

```bash
gh actlock -u
# or
gh actlock --update
```

This will:

1. Find all action files in `.github/` or workflow files in `.github/workflows/`
1. Analyze each file and identify any action or shared workflow references
1. Check if newer versions are available
1. Update the existing full commit SHA to the latest[^1] version's full commit SHA while preserving the original reference in an inline comment

For shared workflows, it converts references like `uses: owner/.github/.github/workflows/file.yml@tag` to use the corresponding SHA while keeping the original tag as a comment.

### Managing Local Cache

The extension maintains a local cache to reduce API calls. You can clear this cache using the `clear` command with the required `-f` or `--force` flag:

> [!NOTE]
> The `-f/--force` flag is required as a safeguard to prevent accidental cache deletion.

```bash
gh actlock clear -f
# or
gh actlock clear --force
```

This will remove the application's cache directory located at:

- **Linux/BSD**: `$XDG_CACHE_HOME/gh-actlock` (typically `~/.cache/gh-actlock`)
- **macOS**: `~/Library/Caches/gh-actlock`
- **Windows**: `%LocalAppData%\gh-actlock` (typically `C:\Users\<username>\AppData\Local\gh-actlock`)

## Limitations

- Only GitHub-hosted actions and shared workflows are pinned (`uses: owner/repo@ref` and `uses: owner/.github/.github/workflows/file.yml@ref`)
- Local actions and Docker actions are skipped
- Requires proper GitHub authentication for higher API rate limits
- Uses the default `yamllint` comment configuration (e.g. two spaces prior to a comment (#), one space after)

## Authentication

> [!TIP]
> For higher REST API rate limits, [create a GitHub token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens) and set `GITHUB_TOKEN` as an environment variable:

```bash
export GITHUB_TOKEN=your_token_here
gh actlock
```

### Upgrade `gh actlock`

```bash
gh ext upgrade actlock
```

### A Note About Latest

When creating a release in GitHub, you have the option to [Set as latest release](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository?tool=webui). This is a mutable tag, that exists at `https://github.com/org/repo/releases/latest`[^2]. Latest can be a bit misleading as it may not be the _highest numerical valued tag_, meaning you could have `v1.0.0`, `v2.0.0` and `v3.0.0` and the `v2.0.0` release could have the `latest` tag set. `actlock` uses the [REST API Endpoint](https://docs.github.com/en/rest/releases/releases?apiVersion=2022-11-28#get-the-latest-release) to get the latest release when passing the `-u/--update` flag. This may not be the desired behavior. In the previously mentioned scenario, it is possible that you may have something like `uses: actions/foo@v3`, but `actions/foo@v2` is tagged `latest` so when passing `-u/--update` to `actlock`, it will be pinned at `uses: actions/foo@sha2 #v2.0.0` instead of `actions/foo@sha3 #v3.0.0`. This behavior is being tracked in issue [#74](https://github.com/esacteksab/gh-actlock/issues/74), I'm not entirely sure how I want to handle this edge case, but I _did_ want to document it here in case you experienced this.

## Examples

### Pinning Actions Example

Before:

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v3
```

After:

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@a81bbbf8298c0fa03ea29cdc473d45769f953675  # v4
      - uses: actions/setup-node@5e21ff4d9bc1a8cf6de233a3057d20ec6b3fb69d  # v3
```

#### Pinning Shared Workflows Example

Before:

```yaml
name: Tools - Check
on:
  pull_request:
    branches:
      - "main"
    paths:
      - "**.go"
      - "**.mod"
      - "**.sum"
      - ".goreleaser.yaml"
concurrency:
  group: ${{ github.workflow }}-${{ github.ref_name }}
  cancel-in-progress: true
permissions:
  contents: read
jobs:
  goreleaser-check-reusable:
    uses: esacteksab/.github/.github/workflows/tools.yml@0.5.3
```

After:

```yaml
name: Tools - Check
on:
  pull_request:
    branches:
      - "main"
    paths:
      - "**.go"
      - "**.mod"
      - "**.sum"
      - ".goreleaser.yaml"
concurrency:
  group: ${{ github.workflow }}-${{ github.ref_name }}
  cancel-in-progress: true
permissions:
  contents: read
jobs:
  goreleaser-check-reusable:
    uses: esacteksab/.github/.github/workflows/tools.yml@7da1f735f5f18ecf049b40ab75503b1191756456  # 0.5.3
```

## Keeping Pinned Actions Updated

You can keep your pinned actions up-to-date using:

- [GitHub Dependabot](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/keeping-your-actions-up-to-date-with-dependabot) - Native GitHub solution for automated updates
- [Renovate](https://docs.renovatebot.com/modules/manager/github-actions/) - Third-party solution with advanced configuration options

These tools will automatically create pull requests to update your pinned SHAs when new versions of actions are released.

## License

MIT Licensed

## Contributing

Contributions welcome! Please feel free to submit a pull request.

### Similar Tools

I built `actlock` because I was manually pinning actions often, and each time I did, I told myself "One day, I will automate this.". The curse of knowing a how to write code, but not to see if someone else already did it, I didn't think to see if there was an existing tool. Only when trying to figure out what to call the tool, did I find a bunch of others. `actlock` may not be the right tool for you, but it doesn't mean that you shouldn't pin your actions to a SHA. These other tools do that (and maybe more!), I have no experience with them, so your mileage my vary.

- [pinact](https://github.com/suzuki-shunsuke/pinact)
- [ratchet](https://github.com/sethvargo/ratchet)
- [pinata](https://github.com/caarlos0/pinata)

[^1]: See [Latest](#a-note-about-latest)

[^2]: [GitHub Docs | Linking to releases](https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases#linking-to-the-latest-release)

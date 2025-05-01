# gh-actlock

`gh-actlock` is a GitHub CLI extension that improves the security of your GitHub Actions workflows by automatically pinning action references to specific commit SHAs.

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
  - uses: actions/checkout@a81bbbf8298c0fa03ea29cdc473d45769f953675 #v4
  - uses: actions/setup-node@8f152de45cc393bb48ce5d89d36b731f54556e65 #main
```

## Installation

### Prerequisites

- [GitHub CLI](https://cli.github.com/) (`gh`) installed

### Install as a GitHub CLI extension

```bash
gh ext install esacteksab/gh-actlock
```

### Upgrade `gh actlock`

```bash
gh ext upgrade actlock
```

## Usage

`gh-actlock` is designed to be run in the root directory of your Git repository. It expects to find a `.github/workflows/` directory containing your workflow files.

Navigate to your repository's root directory and run:

```bash
gh actlock
```

The extension will:

1. Find all workflow files in `.github/workflows/`
1. Analyze each file for GitHub Action references
1. Resolve non-SHA references (tags, branches) to their corresponding commit SHAs
1. Update each workflow file with pinned SHAs, preserving the original reference as a comment

> [!IMPORTANT]
> Make sure you run the command from your repository's root directory where the `.github/workflows/` directory is located.

### Updating Pinned Actions and Shared Workflows

To update actions and shared workflows that are already pinned to SHAs to their latest versions, use the `-u` or `--update` flag:

```bash
gh actlock -u
# or
gh actlock --update
```

This will:

1. Find all workflow files in `.github/workflows/`
1. Identify actions and shared workflows that are already pinned or referenced by tags/versions
1. Check if newer versions are available
1. Update the SHAs to the latest version while preserving the original reference comment

For shared workflows, it converts references like `uses: owner/.github/.github/workflows/file.yml@tag` to use the corresponding SHA while keeping the original tag as a comment.

### Examples

#### Pinning Actions Example

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
      - uses: actions/checkout@a81bbbf8298c0fa03ea29cdc473d45769f953675 #v4
      - uses: actions/setup-node@5e21ff4d9bc1a8cf6de233a3057d20ec6b3fb69d #v3
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
    uses: esacteksab/.github/.github/workflows/tools.yml@7da1f735f5f18ecf049b40ab75503b1191756456 #0.5.3
```

## Authentication

> [!TIP]
> For better rate limits, configure a GitHub token:

```bash
export GITHUB_TOKEN=your_token_here
gh actlock
```

### Managing Cache

The extension maintains a local cache to reduce API calls. You can clear this cache using the `clear` command with the required `-f` or `--force` flag:

```bash
gh actlock clear -f
# or
gh actlock clear --force
```

This will remove the application's cache directory located at:

- **Linux/BSD**: `$XDG_CACHE_HOME/gh-actlock` (typically `~/.cache/gh-actlock`)
- **macOS**: `~/Library/Caches/gh-actlock`
- **Windows**: `%LocalAppData%\gh-actlock` (typically `C:\Users\<username>\AppData\Local\gh-actlock`)

> [!NOTE]
> The `-f/--force` flag is required as a safeguard to prevent accidental cache deletion.

## Commands

- `gh actlock` - Default command to pin actions and shared workflows
- `gh actlock -u` or `gh actlock --update` - Update existing pinned SHAs to latest versions
- `gh actlock clear -f` or `gh actlock clear --force` - Clear the local cache

## Features

- 🔒 Automatically pins GitHub Actions to full commit SHAs
- 🔍 Handles all formats: tags, branches, and already-pinned SHAs
- 💬 Preserves original references as comments
- 📦 Implements HTTP caching to reduce API calls
- 🛠️ Preserves file formatting, indentation, and syntax
- 🔄 Updates pinned SHAs to latest versions with `-u/--update` flag
- 🔗 Pins shared workflow references (`.github/workflows`) to specific commit SHAs
- 🧹 Includes cache management with `clear -f` command

## Limitations

- Only GitHub-hosted actions and shared workflows are pinned (`uses: owner/repo@ref` and `uses: owner/.github/.github/workflows/file.yml@ref`)
- Local actions and Docker actions are skipped
- Requires proper GitHub authentication for API rate limits

## Keeping Pinned Actions Updated

You can keep your pinned actions up-to-date using:

- **`gh actlock -u`** - Use the update flag to update already-pinned SHAs to their latest versions
- [GitHub Dependabot](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/keeping-your-actions-up-to-date-with-dependabot) - Native GitHub solution for automated updates
- [Renovate](https://docs.renovatebot.com/modules/manager/github-actions/) - Third-party solution with advanced configuration options

These tools will automatically create pull requests to update your pinned SHAs when new versions of actions are released.

## License

MIT Licensed

## Contributing

Contributions welcome! Please feel free to submit a Pull Request.

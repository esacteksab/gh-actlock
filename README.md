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
  - uses: actions/checkout@a81bbbf8298c0fa03ea29cdc473d45769f953675 # pinned from v4
  - uses: actions/setup-node@8f152de45cc393bb48ce5d89d36b731f54556e65 # pinned from main
```

## Installation

### Prerequisites

- [GitHub CLI](https://cli.github.com/) (`gh`) installed

### Install as a GitHub CLI extension

```bash
gh extension install esacteksab/gh-actlock
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

### Example

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
      - uses: actions/checkout@a81bbbf8298c0fa03ea29cdc473d45769f953675 # pinned from v4
      - uses: actions/setup-node@5e21ff4d9bc1a8cf6de233a3057d20ec6b3fb69d # pinned from v3
```

## Authentication

> [!TIP]
> For better rate limits, configure a GitHub token:

```bash
export GITHUB_TOKEN=your_token_here
gh actlock
```

## Features

- 🔒 Automatically pins GitHub Actions to full commit SHAs
- 🔍 Handles all formats: tags, branches, and already-pinned SHAs
- 💬 Preserves original references as comments
- 📦 Implements HTTP caching to reduce API calls
- 🛠️ Preserves file formatting, indentation, and syntax

## Limitations

- Only GitHub-hosted actions are pinned (`uses: owner/repo@ref`)
- Local actions and Docker actions are skipped
- Requires proper GitHub authentication for API rate limits
- Does not update actions that are already pinned to SHAs

## Keeping Pinned Actions Updated

`gh-actlock` focuses on the initial pinning of actions to specific commit SHAs for security. It does not update actions that are already pinned to SHAs.

For keeping your pinned actions up-to-date, we recommend using:

- [GitHub Dependabot](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/keeping-your-actions-up-to-date-with-dependabot) - Native GitHub solution for automated updates
- [Renovate](https://docs.renovatebot.com/modules/manager/github-actions/) - Third-party solution with advanced configuration options

These tools will automatically create pull requests to update your pinned SHAs when new versions of actions are released.

> [!NOTE]
> The ability to update existing SHAs with `-u` may be added to `gh actlock` in a future release.

## License

MIT Licensed

## Contributing

Contributions welcome! Please feel free to submit a Pull Request.

#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

git config core.hooksPath .githooks
chmod +x .githooks/pre-commit scripts/check-secrets.sh scripts/install-git-hooks.sh

echo "Installed git hooks from $repo_root/.githooks"

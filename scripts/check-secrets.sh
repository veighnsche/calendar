#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 [--staged|--all]" >&2
}

mode="${1:---staged}"
case "$mode" in
  --staged|--all)
    ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    usage
    exit 2
    ;;
esac

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

blocked_env_regex='(^|/)\.env($|\..+)'
allowed_env_regex='(^|/)\.env\.example$'
sensitive_file_regex='(^|/)(id_(rsa|dsa|ed25519)|.*\.(pem|key|p12|pfx)|.*credentials.*\.json|secrets?\.(yaml|yml|json|txt)|\.npmrc|\.pypirc|\.netrc)$'
secret_value_regex='AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,}|sk-(proj-)?[A-Za-z0-9_-]{20,}|sk_(live|test)_[0-9A-Za-z]{16,}|xox[baprs]-[A-Za-z0-9-]{10,}|-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----|[A-Za-z0-9+.-]+://[^[:space:]/]+:[^[:space:]@]+@'

paths=()
if [[ "$mode" == "--staged" ]]; then
  while IFS= read -r -d '' path; do
    paths+=("$path")
  done < <(git diff --cached --name-only --diff-filter=ACMR -z)
else
  while IFS= read -r -d '' path; do
    [[ -e "$path" ]] || continue
    paths+=("$path")
  done < <(git ls-files -z)
fi

if ((${#paths[@]} == 0)); then
  exit 0
fi

blocked_paths=()
for path in "${paths[@]}"; do
  if [[ "$path" =~ $blocked_env_regex ]] && [[ ! "$path" =~ $allowed_env_regex ]]; then
    blocked_paths+=("$path")
    continue
  fi

  if [[ "$path" =~ $sensitive_file_regex ]]; then
    blocked_paths+=("$path")
  fi
done

if [[ "$mode" == "--staged" ]]; then
  matches="$(
    git diff --cached --unified=0 --no-color --no-ext-diff --diff-filter=ACMR \
      | sed -n '/^\+\+\+/d;/^+/p' \
      | grep -En "$secret_value_regex" || true
  )"
else
  matches="$(grep -EnI "$secret_value_regex" -- "${paths[@]}" || true)"
fi

if ((${#blocked_paths[@]} == 0)) && [[ -z "$matches" ]]; then
  exit 0
fi

echo "Secret scan failed." >&2

if ((${#blocked_paths[@]} > 0)); then
  echo >&2
  echo "Blocked file names:" >&2
  printf '  %s\n' "${blocked_paths[@]}" >&2
fi

if [[ -n "$matches" ]]; then
  echo >&2
  echo "Suspicious content matches:" >&2
  printf '%s\n' "$matches" >&2
fi

echo >&2
echo "Move secrets into an ignored local file such as .env, keep only placeholders in .env.example, then restage." >&2
exit 1

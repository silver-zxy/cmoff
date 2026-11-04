#!/usr/bin/env bash
# Remove game files, binaries, keys and saves from EVERY commit of this repo,
# so the history can be shared. Rewrites all commit hashes (fine for a local
# repo; anyone with an old clone must re-clone).
#
#   tools/clean-history.sh          dry run: list what would be removed
#   tools/clean-history.sh --apply  rewrite history, then shrink .git
#
# Working-tree copies of the files are kept (they are .gitignore'd), so your
# APK, keys and accounts.json stay on disk.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

PATTERNS=('*.apk' '*.apks' '*.xapk' '*.idsig' '*.so' '*.bin' '*.key' '*.keystore'
          '*.jks' 'accounts.json' 'accounts.json.tmp' 'cmserver/cmserver'
          'cmoffline/out' 'out' 'captures' 'game' '*.zip' '*.png' '*.jpg')

list() {
    git rev-list --objects --all |
        git cat-file --batch-check='%(objecttype) %(objectsize) %(rest)' |
        awk '$1 == "blob" && $3 != ""' | sort -u -k3 |
        while read -r _ size path; do
            for p in "${PATTERNS[@]}"; do
                # shellcheck disable=SC2053
                if [[ $path == $p || $(basename "$path") == $p || $path == $p/* ]]; then
                    printf '%10d  %s\n' "$size" "$path"
                    break
                fi
            done
        done
}

echo "files in history that match the clean-up patterns:"
list | tee /tmp/cmoff-clean.txt
[ -s /tmp/cmoff-clean.txt ] || { echo "(none) - nothing to do"; exit 0; }

if [ "${1:-}" != "--apply" ]; then
    echo
    echo "dry run. Re-run with --apply to remove them from every commit."
    exit 0
fi

git diff --quiet && git diff --cached --quiet || { echo "commit or stash your changes first"; exit 1; }

# filter-branch checks out the rewritten HEAD, which deletes tracked copies
# from the working tree: keep them aside and put them back afterwards.
keep=$(mktemp -d)
git ls-files -z -- "${PATTERNS[@]}" | while IFS= read -r -d '' f; do
    mkdir -p "$keep/$(dirname "$f")" && cp -p "$f" "$keep/$f"
done

quoted=$(printf "'%s' " "${PATTERNS[@]}")
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch --force --prune-empty \
    --index-filter "git rm -r --cached --ignore-unmatch -q -- $quoted" \
    --tag-name-filter cat -- --all

cp -rp "$keep"/. . && rm -rf "$keep"
rm -rf .git/refs/original
git reflog expire --expire=now --all
git gc --prune=now --aggressive -q
echo "done. Left in history:"
list || true
echo "repo size: $(du -sh .git | cut -f1)"

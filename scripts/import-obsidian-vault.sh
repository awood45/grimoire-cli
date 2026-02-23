#!/usr/bin/env bash
#
# import-obsidian-vault.sh — Import all markdown files from an Obsidian vault
# into a grimoire. Intended for one-time initial bootstrap.
#
# Usage:
#   ./scripts/import-obsidian-vault.sh <vault-path> <grimoire-path>
#
# Example:
#   ./scripts/import-obsidian-vault.sh ~/my-vault ~/scratch/test-grimoire

set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <vault-path> <grimoire-path>"
    echo ""
    echo "  vault-path         Path to the Obsidian vault directory"
    echo "  grimoire-path     Path to an initialized grimoire"
    exit 1
fi

VAULT_PATH="$1"
BRAIN_PATH="$2"
BINARY="${BINARY:-grimoire-cli}"
FILES_DIR="$BRAIN_PATH/files"

if [[ ! -d "$VAULT_PATH" ]]; then
    echo "Error: Vault path does not exist: $VAULT_PATH"
    exit 1
fi

if [[ ! -d "$BRAIN_PATH" ]]; then
    echo "Error: Grimoire path does not exist: $BRAIN_PATH"
    echo "Run '$BINARY init --path $BRAIN_PATH' first."
    exit 1
fi

imported=0
skipped=0
failed=0

# Find all markdown files, excluding Obsidian internals.
while IFS= read -r -d '' src_file; do
    # Compute the relative path from the vault root.
    rel_path="${src_file#"$VAULT_PATH"/}"

    dest_file="$FILES_DIR/$rel_path"
    dest_dir="$(dirname "$dest_file")"

    # Copy the file, preserving vault folder structure.
    mkdir -p "$dest_dir"
    cp "$src_file" "$dest_file"

    # Derive a tag from the top-level vault folder (if any).
    top_folder="$(echo "$rel_path" | cut -d'/' -f1)"
    tags=()
    if [[ "$top_folder" != "$(basename "$rel_path")" ]]; then
        # Normalize folder name to a tag: lowercase, spaces to hyphens.
        tag="$(echo "$top_folder" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')"
        tags=("$tag")
    fi

    tag_flags=()
    for t in "${tags[@]+"${tags[@]}"}"; do
        tag_flags+=(--tags "$t")
    done

    if "$BINARY" create-file-metadata \
        --path "$BRAIN_PATH" \
        --file "$rel_path" \
        --source-agent "obsidian-import" \
        "${tag_flags[@]+"${tag_flags[@]}"}" \
        > /dev/null 2>&1; then
        echo "  OK  $rel_path"
        ((imported++))
    else
        echo "  FAIL  $rel_path"
        ((failed++))
    fi

done < <(find "$VAULT_PATH" -name '*.md' -not -path '*/.obsidian/*' -not -path '*/.trash/*' -print0 | sort -z)

echo ""
echo "Import complete: $imported imported, $failed failed"

# Quickstart Guide

This guide walks you through installing Grimoire CLI from source and using every command.

## Prerequisites

- **Go 1.21+** installed ([go.dev/dl](https://go.dev/dl/))
- **Ollama** (optional, for similarity search) — [ollama.com](https://ollama.com)

## Install

Clone and install the binary:

```bash
git clone https://github.com/awood45/grimoire-cli.git
cd grimoire-cli
make install
```

This places `grimoire-cli` in `$GOPATH/bin` (usually `~/go/bin`). Make sure that's on your PATH:

```bash
# Add to ~/.zshrc or ~/.bashrc if not already present
export PATH="$HOME/go/bin:$PATH"
```

Verify it works:

```bash
grimoire-cli --help
```

## Tour

### 1. Initialize a grimoire

```bash
grimoire-cli init
```

This creates the default grimoire at `~/.grimoire/` with the directory structure, SQLite database, config, and JSONL ledger.

**Custom path:**

```bash
grimoire-cli init --path ~/my-grimoire
```

**With embeddings** (requires Ollama running with a model pulled):

```bash
ollama pull nomic-embed-text
grimoire-cli init --embedding-provider ollama --embedding-model nomic-embed-text
```

If you already initialized and want to change settings, add `--force`:

```bash
grimoire-cli init --force --embedding-provider ollama --embedding-model nomic-embed-text
```

For all remaining examples, we'll use the default path. Add `--path <dir>` to any command to target a different grimoire.

### 2. Add a file and track it

First, create a markdown file in the grimoire's `files/` directory:

```bash
mkdir -p ~/.grimoire/files/notes
cat > ~/.grimoire/files/notes/my-first-note.md << 'EOF'
# My First Note

This is a note about getting started with Grimoire CLI.
I plan to use it as a shared knowledge store for my AI agents.
EOF
```

Then register its metadata:

```bash
grimoire-cli create-file-metadata \
  --file "notes/my-first-note.md" \
  --source-agent "human" \
  --tags "getting-started" --tags "notes" \
  --summary "Introduction note about setting up the grimoire"
```

The `--file` path is always relative to the `files/` directory.

### 3. Read back metadata

```bash
grimoire-cli get-file-metadata --file "notes/my-first-note.md"
```

Returns the file's tags, source agent, summary, and timestamps as JSON.

### 4. Update metadata

Changed the file? Update its metadata:

```bash
grimoire-cli update-file-metadata \
  --file "notes/my-first-note.md" \
  --tags "getting-started" --tags "notes" --tags "tutorial" \
  --summary "Updated introduction with a walkthrough of all commands"
```

Tags are replaced as a set (pass all tags you want, not just new ones).

### 5. Search

Find files by tag, source agent, date range, or summary content:

```bash
# All files with a specific tag
grimoire-cli search --tag "notes"

# Files matching ANY of several tags
grimoire-cli search --any-tag "notes" --any-tag "tutorial"

# Combine filters: tag + source agent + summary substring
grimoire-cli search --tag "notes" --source-agent "human" --summary-contains "introduction"

# Files updated in a date range
grimoire-cli search --after "2026-01-01T00:00:00Z" --before "2026-12-31T00:00:00Z"

# Control output
grimoire-cli search --tag "notes" --sort updated_at --limit 10
```

### 6. Find similar files

Requires embeddings to be configured (see step 1).

```bash
# Similar to an existing file
grimoire-cli similar --file "notes/my-first-note.md"

# Similar to a text query
grimoire-cli similar --text "how to organize knowledge for AI agents" --limit 5
```

### 7. List tags

See all tags in use and how many files use each one:

```bash
# Alphabetical
grimoire-cli list-tags

# By popularity
grimoire-cli list-tags --sort count
```

### 8. Check health

```bash
grimoire-cli status
```

Reports total files, tracked files, orphaned metadata (pointing to deleted files), untracked files (no metadata), ledger entries, database size, and embedding status.

### 9. Archive a file

Move a file out of the active grimoire into `archive-files/` and remove its metadata:

```bash
grimoire-cli archive-file --file "notes/my-first-note.md"
```

The file is preserved in the archive but no longer appears in searches.

### 10. Rebuild the database

If the SQLite database gets corrupted or out of sync:

```bash
# Fast: replay the JSONL ledger (ledger is source of truth)
grimoire-cli rebuild

# Thorough: walk all files and reconcile (files are source of truth)
grimoire-cli hard-rebuild
```

`rebuild` is fast and trusts the ledger. `hard-rebuild` is slower but catches discrepancies between the filesystem and database.

## Importing from Obsidian

A helper script is included for one-time bulk import from an Obsidian vault:

```bash
./scripts/import-obsidian-vault.sh /path/to/obsidian/vault ~/.grimoire
```

This copies all markdown files (preserving folder structure) and registers metadata for each one, auto-tagging based on the top-level vault folder name.

## Global flags

Every command accepts these flags:

| Flag | Description |
|------|-------------|
| `--path <dir>` | Target a grimoire at a non-default location |
| `--verbose` | Enable verbose output for debugging |

## Tips

- **File paths** passed to `--file` are always relative to the `files/` directory inside the grimoire.
- **Tags** are freeform strings. Use lowercase-kebab-case for consistency (e.g. `meeting-notes`, `deep-research`).
- **Source agent** identifies which agent or person created the file. This makes it easy to filter by origin.
- **Multiple agents** can share a grimoire. Give each agent its own subfolder under `files/` for clean separation, but use tags and search for cross-agent discovery.

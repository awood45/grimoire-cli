# /write-to-grimoire

Write a markdown file into the grimoire and create or update its metadata in a single step.

## Arguments

- **path** (required): Target file path relative to `files/` (e.g., `research/api-notes.md`). Intermediate directories are created automatically.
- **content** (required): The markdown content to write to the file.
- **source_agent** (required): Your agent name (e.g., `claude`, `cursor`).
- **tags** (required): Comma-separated list of tags (e.g., `type/research,lang/go,project/backend`).
- **summary** (optional): A one-line summary of the file contents.

## Instructions

1. **Determine the full file path:**
   - The grimoire base path is: `{{.BasePath}}`
   - The full path is: `{{.BasePath}}/files/<path>`

2. **Create intermediate directories** if they do not exist:
   ```bash
   mkdir -p "$(dirname "{{.BasePath}}/files/<path>")"
   ```

3. **Write the file content** to the full path. Do not include YAML frontmatter — the CLI manages frontmatter automatically.

4. **Check if metadata already exists** for this file:
   ```bash
   grimoire-cli get-file-metadata --file "<path>"
   ```

5. **If metadata does NOT exist** (command returns an error), create it:
   ```bash
   grimoire-cli create-file-metadata --file "<path>" --source-agent "<source_agent>" --tags "<tags>" --summary "<summary>"
   ```

6. **If metadata already exists** (command succeeds), update it:
   ```bash
   grimoire-cli update-file-metadata --file "<path>" --source-agent "<source_agent>" --tags "<tags>" --summary "<summary>"
   ```

7. **Return the metadata** from the create or update command as confirmation.

## Example

```bash
# Write a new research file
mkdir -p "{{.BasePath}}/files/research"

cat > "{{.BasePath}}/files/research/api-design-notes.md" << 'CONTENT'
# API Design Notes

Key decisions for the REST API...
CONTENT

grimoire-cli create-file-metadata \
  --file "research/api-design-notes.md" \
  --source-agent "claude" \
  --tags "type/research,lang/go,project/backend" \
  --summary "Key decisions for the REST API design"
```

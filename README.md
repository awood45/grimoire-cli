# Grimoire

I created this because I found Obsidian's MCPs and available APIs to not quite have the user expeirence I was looking for when building a joint second brain for my agents. I decided to see how well Claude Code, given the right tools and prompting, could build an alternative with the features and agentic user experience I wanted. Grimoire is the result.

This is not an Obsidian replacement. There is no UI for human authoring - it's meant for use by agents.

## Using Grimoire with Agents and Skills

Here is an example of how one of my design document writing skills publishes to Grimoire:

```markdown
### Phase 5: Sync to Grimoire

After writing the local file, sync it to the grimoire so it's searchable and accessible across projects.

1. **Derive the project name** from the project context:
   - Check `design/requirements.md` title or the project's root directory name
   - Use a short, lowercase, hyphenated name (e.g., `my-api-server`, `data-pipeline`)
   - If unclear, use AskUserQuestion to ask the user what to call this project

2. **Write the file** to the grimoire:
   
   ~/.grimoire/files/technical-designs/{project-name}/low-level-design.md
   
   Copy the full contents of `design/low-level-design.md` to this path.

3. **Register metadata** using the `grimoire-cli` CLI:
  
   grimoire-cli create-file-metadata \
     --file "files/technical-designs/{project-name}/low-level-design.md" \
     --source-agent "design-doc-low-level" \
     --tags "type/low-level-design,project/{project-name},topic/technical-design" \
     --summary "Low-level design specification for {project name}"

   If the file already exists in the grimoire, use `update-file-metadata` instead.

4. **If the grimoire CLI is not available** (command not found), skip this phase silently — the local file is the primary output.
```

Installing Grimoire (see QUICKSTART.md) will also populate your Claude code global CLAUDE.md file if you have one, which will help guide agents on how to use Grimoire. A `/write-to-grimoire` skill is also generated for Claude Code.

My recommendation? If you have a Claude Code session read your global CLAUDE.md file and ask it to integrate an agent or skill with Grimoire, it will do a good job.

## Concepts

This application will set up a `~/.grimoire/` folder on the user's machine, which becomes a grimoire available to all agents on the machine. There will be room to expand capabilities but all files will go under a file tree in `~/.grimoire/files/`. Under that folder, there could be further subfolders and files in whatever form the skills and agents on the user's system choose. However, it is generally recommended that different agents each have their own folder, and there could also be shared folders designed to be used by multiple agents (designed carefully - best practice is that usually just one agent can write to a folder, but this is not enforced).

For example, a grimoire supporting an "Executive Assistant" agent and a "Researcher" agent might look like this:

```
~/.grimoire/
└── files/
    ├── intake/                          # Raw inputs dropped here for processing
    │   ├── meeting_notes_2026-02-10.md
    │   ├── article_clipping.md
    │   └── voice_memo_transcript.md
    ├── outbox/                          # Items awaiting human review/action
    │   ├── draft_meeting_invite.md
    │   ├── proposed_asana_task.md
    │   └── email_draft_for_review.md
    ├── daily_calendar/                  # Daily calendar logs
    │   ├── 2026-02-11.md
    │   ├── 2026-02-12.md
    │   └── 2026-02-13.md
    ├── executive_assistant/             # Executive Assistant agent
    │   ├── weekly_plans/
    │   │   ├── 2026-W07.md
    │   │   └── 2026-W08.md
    │   ├── weekly_reviews/
    │   │   ├── 2026-W06_review.md
    │   │   └── 2026-W07_review.md
    │   └── priorities/
    │       ├── active_goals.md
    │       └── blocked_items.md
    └── researcher/                      # Researcher agent
        ├── deep_research_llm_agents.md
        ├── deep_research_market_trends.md
        └── deep_research_competitor_analysis.md
```

We will also produce a CLI, and perhaps eventually an MCP wrapper around the CLI, that has CLI commands to help agents manipulate and search this grimoire. These will include:
- `grimoire-cli init`: Creates the grimoire base folder and structure, and for Claude would add a snippet to ~/.claude/CLAUDE.md noting where this base structure is (in case the user chooses a non-standard path). A file in the base path of the grimoire (for e.g. `~/.grimoire/grimoire.md`) serves as an overall README.
- `grimoire-cli create-file-metadata`: When a file is created, metadata is generated for it. This includes the filepath, a set of string tags (freeform, but there will be a style guide given to agents in grimoire.md - and ideally agents that use the grimoire will be versed in their own commands), and notes the Source Agent name. This will also generate data like a creation timestamp. It will also register that metadata to local databases like SQLite. In the future, it might generate other things like vector embeddings for similarity search. Any database-style changes are recorded in a JSONL ledger so the databases can be recreated as needed.
- `grimoire-cli update-file-metadata`: Intended to be run every time a file is updated. Metadata tags can be changed (set replacement since tags should be in each document), the updated_at timestamp is changed, vector embeddings are regenerated. Any database-style changes are recorded in a JSONL ledger so the databases can be recreated as needed.
- `grimoire-cli get-file-metadata`: Returns the stored metadata for a specific file path (tags, source agent, timestamps, etc.).
- `grimoire-cli delete-file-metadata`: Removes a file's metadata from the database and records the deletion in the JSONL ledger. Should be run when a file is deleted from the grimoire.
- `grimoire-cli search`: Query files by tag, source agent, date range, content type, or combinations thereof. Returns matching file paths and summaries. This is the primary read command agents will use most.
- `grimoire-cli similar`: Find files similar to a given file or text snippet using vector similarity search.
- `grimoire-cli list-tags`: Show all tags currently in use, optionally with counts. Helps agents discover existing tags and avoid creating duplicates.
- `grimoire-cli status`: Health check for the grimoire. Reports file count, orphaned metadata (metadata pointing to files that no longer exist), untracked files (files with no metadata), and database size.
- `grimoire-cli rebuild`: Recreate the SQLite database from the JSONL ledger. This is a fast replay of recorded operations — it trusts the ledger as the source of truth.
- `grimoire-cli hard-rebuild`: Walks every file in the grimoire and regenerates tags and vector embeddings from scratch using file contents as the source of truth (rather than the ledger). Reports any discrepancies found — missing metadata, stale entries, tag changes. The `hard-rebuild` command itself is recorded as the source agent for any metadata records it creates or updates.

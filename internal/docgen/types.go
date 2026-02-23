package docgen

import "time"

// DocData holds the data used to render the grimoire.md document.
type DocData struct {
	TotalFiles     int
	TrackedFiles   int
	OrphanedCount  int
	UntrackedCount int
	TagInventory   []TagEntry
	AgentSummary   []AgentEntry
	LastActivity   time.Time
}

// TagEntry represents a tag and its file count.
type TagEntry struct {
	Name  string
	Count int
}

// AgentEntry represents an agent and its file count.
type AgentEntry struct {
	Name      string
	FileCount int
}

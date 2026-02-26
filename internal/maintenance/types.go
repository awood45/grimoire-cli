// Package maintenance handles health reporting and database reconstruction.
package maintenance

// StatusReport contains the health status of a grimoire.
type StatusReport struct {
	TotalFiles             int
	TrackedFiles           int
	OrphanedCount          int
	UntrackedCount         int
	LedgerEntries          int
	DBSizeBytes            int64
	EmbeddingStatus        string
	EmbeddingSchemaStale   bool   `json:"embedding_schema_stale,omitempty"`
	EmbeddingSchemaMessage string `json:"embedding_schema_message,omitempty"`
}

// RebuildReport summarizes the results of a ledger-based rebuild.
type RebuildReport struct {
	EntriesReplayed  int
	FinalRecordCount int
}

// HardRebuildReport summarizes the results of a file-based hard rebuild.
type HardRebuildReport struct {
	FilesScanned int
	Creates      int
	Updates      int
	Deletes      int
	Warnings     []string
}

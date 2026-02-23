package ledger

// CreatePayload is the payload for "create" operations.
// Contains the full metadata snapshot at creation time.
type CreatePayload struct {
	Tags        []string `json:"tags"`
	Summary     string   `json:"summary,omitempty"`
	SourceAgent string   `json:"source_agent"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// UpdatePayload is the payload for "update" operations.
// Contains a full metadata snapshot (not a delta) so that replay
// can reconstruct state without depending on prior entries.
type UpdatePayload struct {
	Tags        []string `json:"tags"`
	Summary     string   `json:"summary,omitempty"`
	SourceAgent string   `json:"source_agent"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// DeletePayload is the payload for "delete" operations.
// Empty — the filepath and source_agent in the Entry header are sufficient.
type DeletePayload struct{}

// ArchivePayload is the payload for "archive" operations.
// Contains the full original metadata for audit/recovery purposes.
type ArchivePayload struct {
	Tags        []string `json:"tags"`
	Summary     string   `json:"summary,omitempty"`
	SourceAgent string   `json:"source_agent"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	ArchivedTo  string   `json:"archived_to"`
}

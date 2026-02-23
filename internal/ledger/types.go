package ledger

import (
	"encoding/json"
	"time"
)

// Entry represents a single mutation record in the ledger.
type Entry struct {
	Timestamp   time.Time       `json:"timestamp"`
	Operation   string          `json:"operation"`
	Filepath    string          `json:"filepath"`
	SourceAgent string          `json:"source_agent"`
	Payload     json.RawMessage `json:"payload"`
}

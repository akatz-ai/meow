package orchestrator

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateRunID creates a unique run identifier.
// Format: run-{timestamp_hex}-{random_hex}
// Example: run-1a2b3c4d-e5f6a7b8
func GenerateRunID() string {
	ts := time.Now().UnixNano()
	randBytes := make([]byte, 4)
	rand.Read(randBytes)
	return fmt.Sprintf("run-%x-%s", ts, hex.EncodeToString(randBytes))
}

// GenerateExpandedStepID creates a unique step identifier within a run.
// Format: {parent}.{step_id}
// Example: implement.load-context (from expand step "implement")
func GenerateExpandedStepID(parentID, stepID string) string {
	if parentID == "" {
		return stepID
	}
	return parentID + "." + stepID
}

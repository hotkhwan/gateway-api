// internal/services/ingestsvc/types.go
package ingestsvc

// ApproveResult is returned by Approve() after successful Kafka publish.
type ApproveResult struct {
	EventId     string
	KafkaTopic  string
	PublishedAt string
}

// MappingValidationError is returned when required field mappings are missing or invalid.
// Implements the error interface so callers can errors.As() it to extract details.
type MappingValidationError struct {
	MissingTargets []string
	InvalidTargets []string
}

func (e *MappingValidationError) Error() string { return "mapping validation failed" }

// BulkResult is returned by bulk operations.
type BulkResult struct {
	Succeeded []string
	Failed    []BulkFailItem
}

// BulkFailItem describes a single failure within a bulk operation.
type BulkFailItem struct {
	EventId string
	Reason  string
}

package master

import "log"

// OrionmaxInventoryEventService receives Orionmax inventory-change events
// before queue publishing is configured.
type OrionmaxInventoryEventService struct {
	logger *log.Logger
}

// NewOrionmaxInventoryEventService constructs an inventory-event receiver.
func NewOrionmaxInventoryEventService() *OrionmaxInventoryEventService {
	return &OrionmaxInventoryEventService{logger: log.Default()}
}

// ReceiveInventoryChange records an accepted inventory-change event without
// logging its payload. Queue publishing is added in a later flow.
func (service *OrionmaxInventoryEventService) ReceiveInventoryChange(activityType string, rawBody []byte) error {
	service.logger.Printf("Orionmax inventory change received: activity_type=%q bytes=%d", activityType, len(rawBody))
	return nil
}

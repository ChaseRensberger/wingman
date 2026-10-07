package run

import "github.com/chaserensberger/wingman/models"

// ActiveHistory copies the range from the latest completed boundary through the tail.
func ActiveHistory(messages []models.Message, boundaryPart string) []models.Message {
	start := 0
	if boundaryPart != "" {
		for i := len(messages) - 1; i >= 0; i-- {
			message := messages[i]
			if message.State != "" && message.State != models.MessageStateCompleted {
				continue
			}
			for _, part := range message.Content {
				if part.Type() == boundaryPart {
					start = i
					return append([]models.Message(nil), messages[start:]...)
				}
			}
		}
	}
	return append([]models.Message(nil), messages[start:]...)
}

package services

import "go-im/internal/models"

func ToConvType(s string) models.ConversationType {
	switch s {
	case string(models.ConversationTypeGroup):
		return models.ConversationTypeGroup
	default:
		return models.ConversationTypeC2C
	}
}

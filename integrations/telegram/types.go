package telegram

import "github.com/go-telegram/bot/models"

// Re-export SDK types so adapter code imports from one place.
// When the SDK upgrades, any breaking field changes surface here at compile time.
type (
	Update    = models.Update
	Message   = models.Message
	User      = models.User
	Chat      = models.Chat
	PhotoSize = models.PhotoSize
	Document  = models.Document
	Voice     = models.Voice
	Video     = models.Video
	Sticker   = models.Sticker
	Audio     = models.Audio
	Animation = models.Animation

	MessageEntity = models.MessageEntity
)

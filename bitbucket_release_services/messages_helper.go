package bitbucket_release_services

import (
	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/log"
	"time"
)

func SendMessageToTheChannel(channel string, text string) {
	_, _, err := container.C.MessageClient.SendMessage(dto.SlackRequestChatPostMessage{
		Channel:           channel,
		Text:              text,
		AsUser:            true,
		Ts:                time.Time{},
		DictionaryMessage: dto.DictionaryMessage{},
		OriginalMessage:   dto.SlackResponseEventMessage{},
	})
	if err != nil {
		log.Logger().AddError(err).Str("text", text).Msg("Failed to send a message to the channel")
	}
}

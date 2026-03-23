package discord

import (
	"fmt"
	"io"

	"github.com/bwmarrin/discordgo"
	"github.com/yaoapp/yao/attachment"
)

// SendMessage sends a text message to a channel.
func (b *Bot) SendMessage(channelID, text string) (*discordgo.Message, error) {
	return b.session.ChannelMessageSend(channelID, text)
}

// SendMessageReply sends a text message as a reply to another message.
func (b *Bot) SendMessageReply(channelID, text, replyToID string) (*discordgo.Message, error) {
	return b.session.ChannelMessageSendReply(channelID, text, &discordgo.MessageReference{
		MessageID: replyToID,
		ChannelID: channelID,
	})
}

// SendComplex sends a complex message with embeds, files, etc.
func (b *Bot) SendComplex(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
	return b.session.ChannelMessageSendComplex(channelID, data)
}

// SendFile sends a file to a channel.
func (b *Bot) SendFile(channelID, filename string, reader io.Reader) (*discordgo.Message, error) {
	return b.session.ChannelFileSend(channelID, filename, reader)
}

// SendFileWithMessage sends a file with an accompanying text message.
func (b *Bot) SendFileWithMessage(channelID, text, filename string, reader io.Reader) (*discordgo.Message, error) {
	return b.session.ChannelFileSendWithMessage(channelID, text, filename, reader)
}

// SendMediaFromWrapper sends a media file from a Yao attachment wrapper.
func (b *Bot) SendMediaFromWrapper(channelID, wrapper, caption string) error {
	managerName, fileID, err := parseWrapper(wrapper)
	if err != nil {
		return err
	}

	manager, exists := attachment.Managers[managerName]
	if !exists {
		return fmt.Errorf("attachment manager %s not found", managerName)
	}

	resp, err := manager.Download(nil, fileID)
	if err != nil {
		return fmt.Errorf("attachment download %s: %w", fileID, err)
	}
	defer resp.Reader.Close()

	filename := fileID + resp.Extension
	if caption != "" {
		_, err = b.SendFileWithMessage(channelID, caption, filename, resp.Reader)
	} else {
		_, err = b.SendFile(channelID, filename, resp.Reader)
	}
	return err
}

func parseWrapper(wrapper string) (managerName string, fileID string, err error) {
	idx := 0
	for i := range wrapper {
		if wrapper[i] == ':' && i+2 < len(wrapper) && wrapper[i+1] == '/' && wrapper[i+2] == '/' {
			idx = i
			break
		}
	}
	if idx == 0 {
		return "", "", fmt.Errorf("invalid attachment wrapper: %s", wrapper)
	}
	return wrapper[:idx], wrapper[idx+3:], nil
}

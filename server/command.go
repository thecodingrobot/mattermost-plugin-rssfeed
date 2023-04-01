package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

// COMMAND_HELP is the text you see when you type /feed help
const COMMAND_HELP = `* |/feed subscribe url| or |/feed sub url| - Connect your Mattermost channel to an RSS feed 
* |/feed list| - Lists the RSS feeds you have subscribed to
* |/feed unsubscribe url| or |/feed unsub url| - Unsubscribes the Mattermost channel from the RSS feed`

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "feed",
		DisplayName:      "RSSFeed",
		Description:      "Allows user to subscribe to an RSS feed.",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: list, subscribe, sub, unsubscribe, unsub, help",
		AutoCompleteHint: "[command]",
	}
}

func getCommandResponse(responseType, text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: responseType,
		Text:         text,
		Username:     botDisplayName,
		IconURL:      RSSFEED_ICON_URL,
		Type:         model.PostTypeDefault,
	}
}

// ExecuteCommand will execute commands ...
func (p *RSSFeedPlugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {

	split := strings.Fields(args.Command)
	command := split[0]
	parameters := []string{}
	action := ""
	if len(split) > 1 {
		action = split[1]
	}
	if len(split) > 2 {
		parameters = split[2:]
	}

	if command != "/feed" {
		return &model.CommandResponse{}, nil
	}

	switch action {
	case "list":
		txt := "### Subscriptions in this channel\n"
		subscriptions, err := p.getSubscriptions()
		if err != nil {
			return getCommandResponse(model.CommandResponseTypeEphemeral, err.Error()), nil
		}

		for _, value := range subscriptions.Subscriptions {
			if value.ChannelID == args.ChannelId {
				txt += fmt.Sprintf("* `%s`\n", value.URL)
			}
		}
		return getCommandResponse(model.CommandResponseTypeEphemeral, txt), nil
	case "subscribe", "sub":

		if len(parameters) == 0 {
			return getCommandResponse(model.CommandResponseTypeEphemeral, "Please specify a url."), nil
		} else if len(parameters) > 1 {
			return getCommandResponse(model.CommandResponseTypeEphemeral, "Please specify a valid url."), nil
		}

		url := parameters[0]

		if err := p.subscribe(context.Background(), args.ChannelId, url); err != nil {
			return getCommandResponse(model.CommandResponseTypeEphemeral, err.Error()), nil
		}

		return getCommandResponse(model.CommandResponseTypeEphemeral, fmt.Sprintf("Successfully subscribed to %s.", url)), nil
	case "unsubscribe", "unsub":
		if len(parameters) == 0 {
			return getCommandResponse(model.CommandResponseTypeEphemeral, "Please specify a url."), nil
		} else if len(parameters) > 1 {
			return getCommandResponse(model.CommandResponseTypeEphemeral, "Please specify a valid url."), nil
		}

		url := parameters[0]

		if err := p.unsubscribe(args.ChannelId, url); err != nil {
			mlog.Error(err.Error())
			return getCommandResponse(model.CommandResponseTypeEphemeral, "Encountered an error trying to unsubscribe. Please try again."), nil
		}

		return getCommandResponse(model.CommandResponseTypeEphemeral, fmt.Sprintf("Succesfully unsubscribed from %s.", url)), nil
	case "help":
		text := "###### Mattermost RSSFeed Plugin - Slash Command Help\n" + strings.Replace(COMMAND_HELP, "|", "`", -1)
		return getCommandResponse(model.CommandResponseTypeEphemeral, text), nil
	default:
		text := "###### Mattermost RSSFeed Plugin - Slash Command Help\n" + strings.Replace(COMMAND_HELP, "|", "`", -1)
		return getCommandResponse(model.CommandResponseTypeEphemeral, text), nil
	}
}

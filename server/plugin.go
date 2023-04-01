package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	html2md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
	"github.com/mmcdole/gofeed"
)

// RSSFeedPlugin Object
type RSSFeedPlugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	botUserID            string
	processHeartBeatFlag bool
}

// ServeHTTP hook from mattermost plugin
func (p *RSSFeedPlugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	switch path := r.URL.Path; path {
	case "/images/rss.png":
		data, err := os.ReadFile(string("plugins/rssfeed/assets/rss.png"))
		if err == nil {
			w.Header().Set("Content-Type", "image/png")
			w.Write(data)
		} else {
			w.WriteHeader(404)
			w.Write([]byte("404 Something went wrong - " + http.StatusText(404)))
			p.API.LogInfo("/images/rss.png err = ", err.Error())
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		http.NotFound(w, r)
	}
}

func (p *RSSFeedPlugin) setupHeartBeat() {
	heartbeatTime, err := p.getHeartbeatTime()
	if err != nil {
		p.API.LogError(err.Error())
	}

	for p.processHeartBeatFlag {
		//p.API.LogDebug("Heartbeat")

		err := p.processHeartBeat()
		if err != nil {
			p.API.LogError(err.Error())

		}
		time.Sleep(time.Duration(heartbeatTime) * time.Minute)
	}
}

func (p *RSSFeedPlugin) processHeartBeat() error {
	dictionaryOfSubscriptions, err := p.getSubscriptions()
	if err != nil {
		return err
	}

	for _, value := range dictionaryOfSubscriptions.Subscriptions {
		err := p.processSubscription(value)
		if err != nil {
			p.API.LogError(err.Error())
		}
	}

	return nil
}

func (p *RSSFeedPlugin) getHeartbeatTime() (int, error) {
	config := p.getConfiguration()
	heartbeatTime := 5
	var err error
	if len(config.Heartbeat) > 0 {
		heartbeatTime, err = strconv.Atoi(config.Heartbeat)
		if err != nil {
			return 5, err
		}
	}

	return heartbeatTime, nil
}

func fetch(feedURL string, ctx context.Context) (feed string, err error) {
	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}

	if resp != nil {
		defer func() {
			ce := resp.Body.Close()
			if ce != nil {
				err = ce
			}
		}()
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", gofeed.HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// CompareItemsBetweenOldAndNew - This function will used to compare 2 atom xml event objects
// and will return a list of items that are specifically in the newer feed but not in
// the older feed
func CompareItemsBetweenOldAndNew(feedOld *gofeed.Feed, feedNew *gofeed.Feed) []*gofeed.Item {
	itemList := []*gofeed.Item{}

	for _, item1 := range feedNew.Items {
		exists := false
		for _, item2 := range feedOld.Items {
			if item1.GUID == item2.GUID {
				exists = true
				break
			}
		}
		if !exists {
			itemList = append(itemList, item1)
		}
	}
	return itemList
}

func (p *RSSFeedPlugin) processSubscription(subscription *Subscription) error {
	if len(subscription.URL) == 0 {
		return errors.New("no url supplied")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	feedXml, err := fetch(subscription.URL, ctx)
	if err != nil {
		return err
	}

	fp := gofeed.NewParser()
	newFeed, err := fp.ParseString(feedXml)
	if err != nil {
		return err
	}

	// retrieve old xml feed from database
	var oldFeed *gofeed.Feed
	if len(subscription.XML) == 0 {
		oldFeed = &gofeed.Feed{}
	} else {
		oldFeed, err = fp.ParseString(subscription.XML)
		if err != nil {
			return err
		}
	}
	items := CompareItemsBetweenOldAndNew(oldFeed, newFeed)

	// if this is a new subscription only post the latest
	// and not spam the channel
	if len(oldFeed.Items) == 0 && len(items) > 0 {
		items = items[:1]
	}

	for _, item := range items {
		p.createBotPost(subscription.ChannelID, newFeed, item)
	}

	if len(items) > 0 {
		subscription.XML = feedXml
		p.updateSubscription(subscription)
	}

	return nil
}

func (p *RSSFeedPlugin) createBotPost(channelID string, feed *gofeed.Feed, feedItem *gofeed.Item) error {
	converter := html2md.NewConverter("", true, nil)

	description, _ := converter.ConvertString(feedItem.Description)
	post := &model.Post{
		UserId:    p.botUserID,
		ChannelId: channelID,
		Type:      model.PostTypeSlackAttachment,
		Props: map[string]interface{}{
			"attachments": []*model.SlackAttachment{
				{
					Title:      feedItem.Title,
					TitleLink:  feedItem.Link,
					Text:       description,
					Timestamp:  feedItem.Published,
					AuthorName: feed.Title,
					AuthorLink: feed.Link,
				},
			},
		},
	}

	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError(err.Error())
		return err
	}

	return nil
}

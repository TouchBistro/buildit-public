package client

import (
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"
	log "github.com/sirupsen/logrus"
)

type StatsdClient struct {
	client *statsd.Client
}

var sharedClient = &StatsdClient{}

func Statsd() *StatsdClient {
	return sharedClient
}

func (s *StatsdClient) Init(agentHost string) error {
	client, err := statsd.New(
		agentHost,
		// Try creating an unbuffered client to see if completed events show up
		statsd.WithMaxMessagesPerPayload(1),
	)
	if err != nil {
		return err
	}
	s.client = client
	return nil
}

func (s *StatsdClient) Flush() error {
	if s.client == nil {
		return nil
	}
	// Increment metric to test that this stuff is working properly
	err := s.client.Incr("buildit.debug.completed", nil, 1)
	if err != nil {
		log.Debugf("failed to increment test metric: %v", err)
	}
	return s.client.Flush()
}

func (s *StatsdClient) SendEvent(title, text string, tags map[string]string) error {
	if s.client == nil {
		return nil
	}
	statsdTags := make([]string, 0, len(tags))
	for k, v := range tags {
		// Ignore any special Datadog tags
		if k == "com.datadoghq.ad.logs" {
			continue
		}
		statsdTags = append(statsdTags, k+":"+v)
	}
	err := s.client.Event(&statsd.Event{
		Title:          "buildit." + title, // prefix all events so we know they are from buildit
		Text:           text,
		Tags:           statsdTags,
		SourceTypeName: "go",
	})
	if err != nil {
		return fmt.Errorf("failed to create statsd event: %w", err)
	}
	// Also create a gehen event for backwards compatibility until we switch everything to use the new buildit ones
	err = s.client.Event(&statsd.Event{
		Title:          "gehen." + title, // prefix all events so we know they are from buildit
		Text:           text,
		Tags:           statsdTags,
		SourceTypeName: "go",
	})
	if err != nil {
		return fmt.Errorf("failed to create gehen statsd event: %w", err)
	}
	return nil
}

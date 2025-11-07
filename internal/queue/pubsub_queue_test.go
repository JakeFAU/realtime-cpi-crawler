// Package queue_test contains unit tests for the queue package.
package queue_test

import (
	"context"
	"testing"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func TestPubSubProvider_PublishAndClose(t *testing.T) {
	ctx := context.Background()

	// Create a fake Pub/Sub server.
	srv := pstest.NewServer()
	defer srv.Close()

	// Connect to the fake server.
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()

	// Create a client.
	client, err := pubsub.NewClient(ctx, "project-id", option.WithGRPCConn(conn))
	require.NoError(t, err)
	defer client.Close()

	// Create a topic.
	topic, err := client.CreateTopic(ctx, "topic-id")
	require.NoError(t, err)

	// Create a subscription.
	sub, err := client.CreateSubscription(ctx, "sub-id", pubsub.SubscriptionConfig{Topic: topic})
	require.NoError(t, err)

	provider := &queue.PubSubProvider{
		Client: client,
		Topic:  topic,
	}

	// Publish a message.
	crawlID := "test-crawl-id"
	err = provider.Publish(ctx, crawlID)
	require.NoError(t, err)

	// Receive the message.
	c := make(chan *pubsub.Message)
	go func() {
		err := sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
			c <- msg
			msg.Ack()
		})
		require.NoError(t, err)
	}()
	msg := <-c
	assert.Equal(t, crawlID, string(msg.Data))

	// Close the provider.
	err = provider.Close()
	assert.NoError(t, err)
}

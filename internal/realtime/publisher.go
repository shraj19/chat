package realtime

import (
	"context"
	"encoding/json"

	goredis "github.com/redis/go-redis/v9"

	"chat-v2/internal/message"
)

type LocalPublisher struct {
	hub *Hub
}

func NewLocalPublisher(hub *Hub) *LocalPublisher {
	return &LocalPublisher{hub: hub}
}

func (p *LocalPublisher) Publish(ctx context.Context, msg *message.OutMessage) error {
	p.hub.Broadcast(msg)
	return nil
}

type RedisPublisher struct {
	client *goredis.Client
}

func NewRedisPublisher(client *goredis.Client) *RedisPublisher {
	return &RedisPublisher{client: client}
}

func (p *RedisPublisher) Publish(ctx context.Context, msg *message.OutMessage) error {
	if p.client == nil {
		return nil
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, "relay:conversation:"+msg.ConversationID.String(), payload).Err()
}

func (p *RedisPublisher) Subscribe(ctx context.Context, hub *Hub) error {
	if p.client == nil {
		return nil
	}

	sub := p.client.PSubscribe(ctx, "relay:conversation:*")
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case redisMsg, ok := <-sub.Channel():
				if !ok {
					return
				}
				var msg message.OutMessage
				if json.Unmarshal([]byte(redisMsg.Payload), &msg) == nil {
					hub.Broadcast(&msg)
				}
			}
		}
	}()
	return nil
}

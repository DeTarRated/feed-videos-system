package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/domain/social"
)

type Connection struct {
	conn *amqp.Connection
}

type Publisher struct {
	conn     *Connection
	exchange string
	routing  string
}

type Consumer struct {
	conn     *Connection
	queue    string
	exchange string
	routing  string
}

func Open(cfg config.Config) (*Connection, error) {
	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	return &Connection{conn: conn}, nil
}

func (c *Connection) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Connection) Channel() (*amqp.Channel, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("rabbitmq connection is nil")
	}
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	return ch, nil
}

func NewPublisher(conn *Connection, exchange, routingKey string) *Publisher {
	return &Publisher{conn: conn, exchange: exchange, routing: routingKey}
}

func NewConsumer(conn *Connection, queue, exchange, routingKey string) *Consumer {
	return &Consumer{conn: conn, queue: queue, exchange: exchange, routing: routingKey}
}

func (p *Publisher) PublishCommentEvent(ctx context.Context, event social.Event) error {
	if p == nil || p.conn == nil {
		return nil
	}
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := DeclareExchange(ch, p.exchange); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return PublishJSON(ctx, ch, p.exchange, p.routing, body)
}

func (p *Publisher) PublishLikeEvent(ctx context.Context, event social.Event) error {
	if p == nil || p.conn == nil {
		return nil
	}
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := DeclareExchange(ch, p.exchange); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return PublishJSON(ctx, ch, p.exchange, p.routing, body)
}

func (c *Consumer) Setup() (string, error) {
	if c == nil || c.conn == nil {
		return "", fmt.Errorf("rabbitmq connection is nil")
	}
	ch, err := c.conn.Channel()
	if err != nil {
		return "", err
	}
	defer ch.Close()

	if err := DeclareExchange(ch, c.exchange); err != nil {
		return "", err
	}
	queue, err := DeclareQueue(ch, c.queue)
	if err != nil {
		return "", err
	}
	if err := ch.QueueBind(queue.Name, c.routing, c.exchange, false, nil); err != nil {
		return "", err
	}
	return queue.Name, nil
}

func DeclareQueue(ch *amqp.Channel, queue string) (amqp.Queue, error) {
	return ch.QueueDeclare(queue, true, false, false, false, nil)
}

func DeclareExchange(ch *amqp.Channel, exchange string) error {
	return ch.ExchangeDeclare(exchange, amqp.ExchangeTopic, true, false, false, false, nil)
}

func PublishJSON(ctx context.Context, ch *amqp.Channel, exchange, routingKey string, body []byte) error {
	return ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
}

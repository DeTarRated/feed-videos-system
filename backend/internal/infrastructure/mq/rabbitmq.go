package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/domain/social"
)

type Connection struct {
	conn *amqp.Connection
}

type Publisher struct {
	conn      *Connection
	exchange  string
	routing   string
	channel   *amqp.Channel
	confirms  chan amqp.Confirmation
	notifyErr chan *amqp.Error
	mu        sync.Mutex
	publishMu sync.Mutex
	pending   map[uint64]chan amqp.Confirmation
	done      chan struct{}
	closeOnce sync.Once
}

type Consumer struct {
	conn     *Connection
	queue    string
	exchange string
	routing  string
}

func (p *Publisher) Channel() *amqp.Channel { return p.channel }
func (p *Publisher) Confirms() <-chan amqp.Confirmation {
	return p.confirms
}
func (p *Publisher) NotifyClose() <-chan *amqp.Error {
	return p.notifyErr
}

func (p *Publisher) PublishCommand(ctx context.Context, routingKey string, body []byte) error {
	if p == nil || p.conn == nil {
		return fmt.Errorf("rabbitmq publisher is nil")
	}
	if routingKey == "" {
		routingKey = p.routing
	}
	return PublishJSON(ctx, p.channel, p.exchange, routingKey, body)
}

func (p *Publisher) Close() error {
	if p == nil || p.channel == nil {
		return nil
	}
	p.closeOnce.Do(func() { close(p.done) })
	return p.channel.Close()
}

func (p *Publisher) consumeConfirmations() {
	for {
		select {
		case <-p.done:
			return
		case err, ok := <-p.notifyErr:
			if !ok {
				return
			}
			p.failAll(err)
		case confirmation, ok := <-p.confirms:
			if !ok {
				return
			}
			p.mu.Lock()
			ch, exists := p.pending[confirmation.DeliveryTag]
			if exists {
				delete(p.pending, confirmation.DeliveryTag)
			}
			p.mu.Unlock()
			if exists {
				ch <- confirmation
				close(ch)
			}
		}
	}
}

func (p *Publisher) failAll(cause error) {
	p.mu.Lock()
	pending := p.pending
	p.pending = make(map[uint64]chan amqp.Confirmation)
	p.mu.Unlock()
	for _, ch := range pending {
		ch <- amqp.Confirmation{Ack: false}
		close(ch)
	}
	_ = cause
}

func (p *Publisher) PublishCommandWithConfirm(ctx context.Context, routingKey string, body []byte, timeout time.Duration) error {
	if p == nil || p.conn == nil {
		return fmt.Errorf("rabbitmq publisher is nil")
	}
	if routingKey == "" {
		routingKey = p.routing
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	confirmationCh := make(chan amqp.Confirmation, 1)
	p.publishMu.Lock()
	seqNo := p.channel.GetNextPublishSeqNo()
	p.mu.Lock()
	p.pending[seqNo] = confirmationCh
	p.mu.Unlock()

	err := PublishJSON(ctx, p.channel, p.exchange, routingKey, body)
	p.publishMu.Unlock()
	if err != nil {
		p.mu.Lock()
		delete(p.pending, seqNo)
		p.mu.Unlock()
		return err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case confirmation := <-confirmationCh:
		if !confirmation.Ack {
			return errors.New("rabbitmq publisher confirm nack")
		}
		return nil
	case <-timer.C:
		p.mu.Lock()
		delete(p.pending, seqNo)
		p.mu.Unlock()
		return fmt.Errorf("rabbitmq publisher confirm timeout")
	case <-ctx.Done():
		p.mu.Lock()
		delete(p.pending, seqNo)
		p.mu.Unlock()
		return ctx.Err()
	}
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

func NewPublisher(conn *Connection, exchange, routingKey string) (*Publisher, error) {
	if conn == nil || conn.conn == nil {
		return nil, fmt.Errorf("rabbitmq connection is nil")
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := DeclareExchange(ch, exchange); err != nil {
		_ = ch.Close()
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, err
	}
	p := &Publisher{
		conn:      conn,
		exchange:  exchange,
		routing:   routingKey,
		channel:   ch,
		confirms:  ch.NotifyPublish(make(chan amqp.Confirmation, 32)),
		notifyErr: ch.NotifyClose(make(chan *amqp.Error, 1)),
		pending:   make(map[uint64]chan amqp.Confirmation),
		done:      make(chan struct{}),
	}
	go p.consumeConfirmations()
	return p, nil
}

func NewConsumer(conn *Connection, queue, exchange, routingKey string) *Consumer {
	return &Consumer{conn: conn, queue: queue, exchange: exchange, routing: routingKey}
}

func (p *Publisher) PublishCommentEvent(ctx context.Context, event social.Event) error {
	if p == nil || p.conn == nil {
		return fmt.Errorf("rabbitmq publisher is nil")
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return PublishJSON(ctx, p.channel, p.exchange, p.routing, body)
}

func (p *Publisher) PublishLikeEvent(ctx context.Context, event social.Event) error {
	if p == nil || p.conn == nil {
		return fmt.Errorf("rabbitmq publisher is nil")
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return PublishJSON(ctx, p.channel, p.exchange, p.routing, body)
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

func DeclareCommandTopology(ch *amqp.Channel, cfg CommandTopologyConfig) error {
	if err := DeclareExchange(ch, cfg.Exchange); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(cfg.DLQ, true, false, false, false, nil); err != nil {
		return err
	}

	likeRetryArgs := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.DLQ,
		"x-message-ttl":             int64(cfg.RetryTTL / time.Millisecond),
	}
	if _, err := ch.QueueDeclare(cfg.RetryQueue+".like", true, false, false, false, likeRetryArgs); err != nil {
		return err
	}
	commentRetryArgs := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.DLQ,
		"x-message-ttl":             int64(cfg.RetryTTL / time.Millisecond),
	}
	if _, err := ch.QueueDeclare(cfg.RetryQueue+".comment", true, false, false, false, commentRetryArgs); err != nil {
		return err
	}

	likeArgs := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.RetryQueue + ".like",
	}
	likeQueue, err := ch.QueueDeclare(cfg.LikeQueue, true, false, false, false, likeArgs)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(likeQueue.Name, cfg.LikeRoutingKey, cfg.Exchange, false, nil); err != nil {
		return err
	}

	commentArgs := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.RetryQueue + ".comment",
	}
	commentQueue, err := ch.QueueDeclare(cfg.CommentQueue, true, false, false, false, commentArgs)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(commentQueue.Name, cfg.CommentRoutingKey, cfg.Exchange, false, nil); err != nil {
		return err
	}
	return nil
}

type CommandTopologyConfig struct {
	Exchange          string
	LikeQueue         string
	LikeRoutingKey    string
	CommentQueue      string
	CommentRoutingKey string
	RetryQueue        string
	RetryTTL          time.Duration
	DLQ               string
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

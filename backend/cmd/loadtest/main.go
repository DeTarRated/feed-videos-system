package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	BaseURL      string
	VideoID      int
	Users        int
	Duration     time.Duration
	Workers      int
	CommentRatio float64
	JWTSecret    string
	UserIDStart  int
	SeedOnly     bool
	Mode         string
}

type Metrics struct {
	mu              sync.Mutex
	startTime       time.Time
	latency         []time.Duration
	commitLatency   []time.Duration // accepted -> outcome visible in DB
	acceptedLatency []time.Duration
	readLatency     []time.Duration
	accepted        int
	errors          int
	status          map[int]int
}

func (m *Metrics) begin() {
	m.mu.Lock()
	m.startTime = time.Now()
	m.mu.Unlock()
}

func (m *Metrics) record(d time.Duration, statusCode int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.errors++
		return
	}
	m.status[statusCode]++
	if statusCode >= 200 && statusCode < 300 {
		m.latency = append(m.latency, d)
	}
	if statusCode == http.StatusAccepted {
		m.accepted++
		m.acceptedLatency = append(m.acceptedLatency, d)
	}
	if statusCode == http.StatusOK {
		m.readLatency = append(m.readLatency, d)
	}
}

func (m *Metrics) report() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	elapsed := time.Since(m.startTime).Seconds()
	var qps float64
	if elapsed > 0 {
		qps = float64(m.accepted) / elapsed
	}
	sort.Slice(m.latency, func(i, j int) bool { return m.latency[i] < m.latency[j] })
	sort.Slice(m.acceptedLatency, func(i, j int) bool { return m.acceptedLatency[i] < m.acceptedLatency[j] })
	sort.Slice(m.readLatency, func(i, j int) bool { return m.readLatency[i] < m.readLatency[j] })
	pct := func(ds []time.Duration, p int) time.Duration {
		if len(ds) == 0 {
			return 0
		}
		return ds[len(ds)*p/100]
	}
	return map[string]any{
		"errors":                  m.errors,
		"accepted_202":            m.accepted,
		"accepted_qps":            qps,
		"status_counts":           m.status,
		"success_latency_p50_ms":  pct(m.latency, 50).Milliseconds(),
		"success_latency_p95_ms":  pct(m.latency, 95).Milliseconds(),
		"success_latency_p99_ms":  pct(m.latency, 99).Milliseconds(),
		"accepted_latency_p50_ms": pct(m.acceptedLatency, 50).Milliseconds(),
		"accepted_latency_p95_ms": pct(m.acceptedLatency, 95).Milliseconds(),
		"accepted_latency_p99_ms": pct(m.acceptedLatency, 99).Milliseconds(),
		"read_latency_p50_ms":     pct(m.readLatency, 50).Milliseconds(),
		"read_latency_p95_ms":     pct(m.readLatency, 95).Milliseconds(),
		"read_latency_p99_ms":     pct(m.readLatency, 99).Milliseconds(),
	}
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BaseURL, "url", "http://localhost:18080/api/v1", "API base URL")
	flag.IntVar(&cfg.VideoID, "video", 1, "Target video ID")
	flag.IntVar(&cfg.Users, "users", 10, "Pre-registered user count")
	flag.DurationVar(&cfg.Duration, "duration", 30*time.Second, "Test duration")
	flag.IntVar(&cfg.Workers, "workers", 20, "Concurrent workers")
	flag.Float64Var(&cfg.CommentRatio, "comment-ratio", 0.15, "Comment ratio (0-1)")
	flag.StringVar(&cfg.JWTSecret, "jwt-secret", "", "If set, mint tokens locally for user IDs instead of register/login (bypasses auth rate limits)")
	flag.IntVar(&cfg.UserIDStart, "user-id-start", 1, "First user ID when -jwt-secret is set")
	flag.BoolVar(&cfg.SeedOnly, "seed-only", false, "Only mint tokens and exit (sanity check)")
	flag.StringVar(&cfg.Mode, "mode", "mixed", "Traffic mode: mixed | read | like | comment")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+60*time.Second)
	defer cancel()

	transport := &http.Transport{
		MaxIdleConns:        cfg.Workers * 2,
		MaxIdleConnsPerHost: cfg.Workers * 2,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}

	tokens := make([]string, cfg.Users)
	if cfg.JWTSecret != "" {
		for i := 0; i < cfg.Users; i++ {
			token, err := mintToken(cfg.JWTSecret, uint(cfg.UserIDStart+i))
			if err != nil {
				fmt.Fprintf(os.Stderr, "mint token %d: %v\n", i, err)
				continue
			}
			tokens[i] = token
		}
	} else {
		for i := 0; i < cfg.Users; i++ {
			username := fmt.Sprintf("loaduser%d", i)
			token, err := loginUser(client, cfg.BaseURL, username)
			if err != nil {
				fmt.Fprintf(os.Stderr, "user %d login failed: %v\n", i, err)
				continue
			}
			tokens[i] = token
		}
	}
	if cfg.SeedOnly {
		fmt.Printf("minted %d tokens (user ids %d..%d)\n", cfg.Users, cfg.UserIDStart, cfg.UserIDStart+cfg.Users-1)
		return
	}

	deadline := time.Now().Add(cfg.Duration)
	metrics := &Metrics{status: make(map[int]int)}
	metrics.begin()

	// Poll for outcome commit latency on a sample of accepted operations
	var pollToken string
	for _, t := range tokens {
		if t != "" {
			pollToken = t
			break
		}
	}
	_ = pollToken
	sem := make(chan struct{}, cfg.Workers)
	var wg sync.WaitGroup

	toggleCounters := make([]*atomic.Int64, cfg.Users)
	for i := range toggleCounters {
		toggleCounters[i] = &atomic.Int64{}
	}
	userIdx := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			goto done
		default:
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			token := tokens[idx%len(tokens)]
			if token == "" {
				return
			}
			start := time.Now()
			var statusCode int
			var err error
			switch cfg.Mode {
			case "read":
				statusCode, err = doRead(client, cfg.BaseURL, cfg.VideoID, idx)
			case "like":
				userKey := idx % len(tokens)
				count := toggleCounters[userKey].Add(1)
				statusCode, err = doLikeToggle(client, cfg.BaseURL, token, cfg.VideoID, userKey, int(count))
			case "comment":
				statusCode, err = doComment(client, cfg.BaseURL, token, cfg.VideoID, idx)
			default:
				r := float64(idx%1000) / 1000.0
				switch {
				case r < cfg.CommentRatio:
					statusCode, err = doComment(client, cfg.BaseURL, token, cfg.VideoID, idx)
				case r < cfg.CommentRatio+0.1:
					statusCode, err = doRead(client, cfg.BaseURL, cfg.VideoID, idx)
				default:
					userKey := idx % len(tokens)
					count := toggleCounters[userKey].Add(1)
					statusCode, err = doLikeToggle(client, cfg.BaseURL, token, cfg.VideoID, userKey, int(count))
				}
			}
			metrics.record(time.Since(start), statusCode, err)
		}(userIdx)
		userIdx++
	}

done:
	wg.Wait()
	report := metrics.report()
	report["config"] = map[string]any{
		"video_id":      cfg.VideoID,
		"users":         cfg.Users,
		"workers":       cfg.Workers,
		"duration":      cfg.Duration.String(),
		"comment_ratio": cfg.CommentRatio,
		"mode":          cfg.Mode,
		"base_url":      cfg.BaseURL,
	}
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
}

type jwtClaims struct {
	UserID uint `json:"userId"`
	jwt.RegisteredClaims
}

func mintToken(secret string, userID uint) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func loginUser(client *http.Client, baseURL, username string) (string, error) {
	password := "password123"
	registerBody, _ := json.Marshal(map[string]string{"username": username, "password": password, "nickname": username})
	client.Post(baseURL+"/auth/register", "application/json", bytes.NewReader(registerBody))

	loginBody, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := client.Post(baseURL+"/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Data.Token, nil
}

func doLikeToggle(client *http.Client, baseURL, token string, videoID, userKey, toggleCount int) (int, error) {
	method := http.MethodPost
	if toggleCount%2 == 0 {
		method = http.MethodDelete
	}
	req, _ := http.NewRequest(method, fmt.Sprintf("%s/videos/%d/like", baseURL, videoID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", fmt.Sprintf("load-like-%d-%d", userKey, time.Now().UnixNano()))
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func doRead(client *http.Client, baseURL string, videoID, idx int) (int, error) {
	var path string
	if idx%2 == 0 {
		path = fmt.Sprintf("%s/feed/hot?limit=10", baseURL)
	} else {
		path = fmt.Sprintf("%s/videos/%d", baseURL, videoID)
	}
	resp, err := client.Get(path)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func doComment(client *http.Client, baseURL, token string, videoID, idx int) (int, error) {
	body, _ := json.Marshal(map[string]string{"content": fmt.Sprintf("load test comment %d", idx)})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/videos/%d/comments", baseURL, videoID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("load-comment-%d-%d", idx, time.Now().UnixNano()))
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

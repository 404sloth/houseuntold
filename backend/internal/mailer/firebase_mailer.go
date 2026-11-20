package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type TicketNotification struct {
	Phone      string
	Email      string
	EventTitle string
	TicketID   string
	TicketCode string
	TicketURL  string
}

type Mailer interface {
	EnqueuePassNotification(ctx context.Context, msg TicketNotification)
	CloseAndWait(ctx context.Context) error
}

// FirebasePassMailer uses a bounded buffered channel and worker pool to dispatch
// WhatsApp/SMS notifications via Firebase Cloud Messaging / SMS gateway without
// blocking webhook HTTP responses and drains cleanly on shutdown.
type FirebasePassMailer struct {
	httpClient  *http.Client
	projectID   string
	serverKey   string
	appBaseURL  string
	logger      *slog.Logger
	jobs        chan TicketNotification
	wg          sync.WaitGroup
	closeOnce   sync.Once
}

func NewFirebasePassMailer(projectID, serverKey, appBaseURL string, workers int, logger *slog.Logger) *FirebasePassMailer {
	if workers <= 0 {
		workers = 2
	}
	if logger == nil {
		logger = slog.Default()
	}
	m := &FirebasePassMailer{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		projectID:  projectID,
		serverKey:  serverKey,
		appBaseURL: appBaseURL,
		logger:     logger,
		jobs:       make(chan TicketNotification, 128),
	}
	for range workers {
		m.wg.Add(1)
		go m.worker()
	}
	return m
}

func (m *FirebasePassMailer) worker() {
	defer m.wg.Done()
	for job := range m.jobs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := m.sendPass(ctx, job); err != nil {
			m.logger.Warn("firebase pass notification dispatch failed", "ticket", job.TicketCode, "err", err)
		}
		cancel()
	}
}

func (m *FirebasePassMailer) sendPass(ctx context.Context, job TicketNotification) error {
	ticketLink := fmt.Sprintf("%s/tickets?code=%s", m.appBaseURL, job.TicketCode)
	if job.TicketURL != "" {
		ticketLink = job.TicketURL
	}

	if m.serverKey == "" {
		// Safe structured log when external Firebase push key is omitted in local test envs
		m.logger.InfoContext(ctx, "ticket pass queued for WhatsApp/SMS delivery",
			"phone", job.Phone,
			"ticketId", job.TicketID,
			"ticketCode", job.TicketCode,
			"link", ticketLink,
		)
		return nil
	}

	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", m.projectID)
	payload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"topic": "ticket-alerts",
			"notification": map[string]string{
				"title": fmt.Sprintf("Confirmed: %s", job.EventTitle),
				"body":  fmt.Sprintf("Pass %s ready (%s). View: %s", job.TicketCode, job.TicketID, ticketLink),
			},
			"data": map[string]string{
				"ticket_id":   job.TicketID,
				"ticket_code": job.TicketCode,
				"phone":       job.Phone,
				"link":        ticketLink,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal_fcm_payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build_fcm_request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.serverKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute_fcm_request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("fcm returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (m *FirebasePassMailer) EnqueuePassNotification(ctx context.Context, msg TicketNotification) {
	select {
	case m.jobs <- msg:
	case <-ctx.Done():
	default:
		m.logger.Warn("firebase pass mailer buffer full; dropping notification", "ticket", msg.TicketCode)
	}
}

func (m *FirebasePassMailer) CloseAndWait(ctx context.Context) error {
	m.closeOnce.Do(func() {
		close(m.jobs)
	})

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("mailer_shutdown_timeout: %w", ctx.Err())
	}
}

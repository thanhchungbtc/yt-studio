package services

import (
	"sync"
	"time"

	"github.com/tbui/yt-studio/internal/domain/provider"
)

// consoleFlush batches console frames while a model streams.
const consoleFlush = 50 * time.Millisecond

// LLMFrame is a run's text so far (backlog) or since the last frame.
type LLMFrame struct {
	Run       uint64    `json:"run"`
	VideoID   string    `json:"videoId"`
	Label     string    `json:"label"`
	Model     string    `json:"model"`
	Text      string    `json:"text,omitempty"`
	Done      bool      `json:"done,omitempty"`
	Error     string    `json:"error,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	Millis    int64     `json:"ms,omitempty"`
}

func llmFrameFrom(f provider.LLMFrame) LLMFrame {
	out := LLMFrame{
		Run:       f.Run,
		VideoID:   f.Video.String(),
		Label:     f.Label,
		Model:     f.Model,
		Text:      f.Text,
		Done:      f.Done,
		Truncated: f.Truncated,
		StartedAt: f.StartedAt,
		Millis:    f.Duration.Milliseconds(),
	}
	if f.Err != nil {
		out.Error = f.Err.Error()
	}
	return out
}

// ConsoleBatch is a ConsoleEvent; a session's first batch is its backlog.
type ConsoleBatch struct {
	Session int        `json:"session"`
	Reset   bool       `json:"reset,omitempty"`
	Frames  []LLMFrame `json:"frames"`
}

// ConsoleSource is the LLM log's subscription side.
type ConsoleSource interface {
	Subscribe() ([]provider.LLMFrame, <-chan provider.LLMFrame, func())
}

// ConsoleService streams model exchanges only while the console is open.
type ConsoleService struct {
	source ConsoleSource
	emit   Emitter

	mu      sync.Mutex
	session int // the newest session opened
	closed  int // the newest session closed
	cancel  func()
}

// NewConsoleService creates a ConsoleService.
func NewConsoleService(source ConsoleSource, emit Emitter) *ConsoleService {
	return &ConsoleService{source: source, emit: emit}
}

// Open streams to session; calls for sessions older than the newest seen are ignored.
func (s *ConsoleService) Open(session int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session <= s.session || session <= s.closed {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	backlog, frames, cancel := s.source.Subscribe()
	s.session, s.cancel = session, cancel
	go s.forward(session, backlog, frames)
}

// Close stops sending session's frames.
func (s *ConsoleService) Close(session int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = max(s.closed, session)
	if s.cancel != nil && s.session == session {
		s.cancel()
		s.cancel = nil
	}
}

func (s *ConsoleService) forward(session int, backlog []provider.LLMFrame, frames <-chan provider.LLMFrame) {
	batch := make([]LLMFrame, 0, len(backlog)+16)
	for _, f := range backlog {
		batch = append(batch, llmFrameFrom(f))
	}
	// Sent even when empty: it's what tells the console it's connected.
	s.emit.Emit(ConsoleEvent, ConsoleBatch{Session: session, Reset: true, Frames: batch})
	batch = nil

	ticker := time.NewTicker(consoleFlush)
	defer ticker.Stop()
	for {
		select {
		case f, open := <-frames:
			if !open {
				return
			}
			batch = append(batch, llmFrameFrom(f))
		case <-ticker.C:
			if len(batch) > 0 {
				s.emit.Emit(ConsoleEvent, ConsoleBatch{Session: session, Frames: batch})
				batch = nil
			}
		}
	}
}

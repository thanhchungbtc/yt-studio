package sqlite

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

var (
	_ repository.TaskReader = (*Store)(nil)
	_ repository.TaskWriter = (*Store)(nil)
)

// TaskByID reads one task.
func (s *Store) TaskByID(ctx context.Context, id entity.TaskID) (entity.Task, error) {
	row, err := s.rq.GetTaskByID(ctx, string(id))
	if err != nil {
		return entity.Task{}, wrapNotFound(err, "task", string(id))
	}
	return taskFromRow(row), nil
}

// ListTasksByVideo reads a video's tasks in DAG order.
func (s *Store) ListTasksByVideo(ctx context.Context, videoID entity.VideoID) ([]entity.Task, error) {
	rows, err := s.rq.ListTasksByVideo(ctx, string(videoID))
	if err != nil {
		return nil, fmt.Errorf("list tasks of %s: %w", videoID, err)
	}
	out := make([]entity.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, taskFromRow(r))
	}
	return out, nil
}

// ListRecentTasks powers the operator console's live table.
func (s *Store) ListRecentTasks(ctx context.Context, limit int) ([]entity.Task, error) {
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.rq.ListRecentTasks(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("list recent tasks: %w", err)
	}
	out := make([]entity.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, taskFromRow(r))
	}
	return out, nil
}

// CountTasksByVideo aggregates a video's task census in one indexed query,
// rather than loading 305 rows to count them.
func (s *Store) CountTasksByVideo(ctx context.Context, videoID entity.VideoID) (repository.TaskCounts, error) {
	row, err := s.rq.CountTasksByVideo(ctx, string(videoID))
	if err != nil {
		return repository.TaskCounts{}, fmt.Errorf("count tasks of %s: %w", videoID, err)
	}
	return repository.TaskCounts{
		Total:            int(row.Total),
		Succeeded:        int(row.Succeeded),
		Failed:           int(row.Failed),
		Running:          int(row.Running),
		Ready:            int(row.Ready),
		Blocked:          int(row.Blocked),
		AwaitingApproval: int(row.Awaiting),
		Cancelled:        int(row.Cancelled),
		Stale:            int(row.Stale),
	}, nil
}

// ListOpenGraphs reloads every video that still has open tasks, with its edges,
// so the server resumes rather than restarts.
func (s *Store) ListOpenGraphs(ctx context.Context) ([]repository.VideoGraph, error) {
	videoIDs, err := s.rq.ListVideosWithOpenTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open videos: %w", err)
	}
	out := make([]repository.VideoGraph, 0, len(videoIDs))
	for _, id := range videoIDs {
		g, err := s.GraphByVideo(ctx, entity.VideoID(id))
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

// GraphByVideo reloads one video's DAG regardless of what state its tasks are
// in, which is what resuming a video the loop has forgotten needs.
func (s *Store) GraphByVideo(ctx context.Context, videoID entity.VideoID) (repository.VideoGraph, error) {
	tasks, err := s.ListTasksByVideo(ctx, videoID)
	if err != nil {
		return repository.VideoGraph{}, err
	}
	depRows, err := s.rq.ListTaskDepsByVideo(ctx, string(videoID))
	if err != nil {
		return repository.VideoGraph{}, fmt.Errorf("list deps of %s: %w", videoID, err)
	}
	edges := make([]repository.TaskEdge, 0, len(depRows))
	for _, d := range depRows {
		edges = append(edges, repository.TaskEdge{
			VideoID: entity.VideoID(d.VideoID),
			From:    entity.TaskID(d.FromID),
			To:      entity.TaskID(d.ToID),
		})
	}
	return repository.VideoGraph{VideoID: videoID, Tasks: tasks, Edges: edges}, nil
}

// InsertGraph writes a whole DAG in one transaction. Task ids are
// deterministic, so re-enqueueing an existing video inserts nothing.
func (s *Store) InsertGraph(ctx context.Context, videoID entity.VideoID, tasks []entity.Task, edges []repository.TaskEdge) error {
	return s.doTx(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		for i := range tasks {
			t := &tasks[i]
			if err := q.InsertTask(ctx, sqlcgen.InsertTaskParams{
				ID:            string(t.ID),
				VideoID:       string(t.VideoID),
				ChapterID:     chapterIDPtr(t.ChapterID),
				Kind:          string(t.Kind),
				Ordinal:       int64(t.Ordinal),
				Idx:           int64(t.Index),
				State:         string(t.State),
				Pool:          string(t.Pool),
				Gate:          string(t.Gate),
				Attempt:       int64(t.Attempt),
				MaxAttempts:   int64(t.MaxAttempts),
				DepsRemaining: int64(t.DepsRemaining),
				Error:         t.Error,
				Stale:         boolToInt(t.Stale),
				CreatedAt:     toUnix(t.CreatedAt),
				UpdatedAt:     toUnix(t.UpdatedAt),
				StartedAt:     toUnixPtr(t.StartedAt),
				FinishedAt:    toUnixPtr(t.FinishedAt),
				NotBefore:     toUnixPtr(t.NotBefore),
			}); err != nil {
				return fmt.Errorf("insert task %s: %w", t.ID, err)
			}
		}
		for _, e := range edges {
			if err := q.InsertTaskDep(ctx, sqlcgen.InsertTaskDepParams{
				VideoID: string(e.VideoID),
				FromID:  string(e.From),
				ToID:    string(e.To),
			}); err != nil {
				return fmt.Errorf("insert dep %s->%s: %w", e.From, e.To, err)
			}
		}
		return nil
	})
}

// ApplyTransitions commits N transitions in a single transaction, which is what
// makes a burst of simultaneous completions one write rather than N.
func (s *Store) ApplyTransitions(ctx context.Context, transitions []repository.TaskTransition) error {
	if len(transitions) == 0 {
		return nil
	}
	return s.doTx(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		for i := range transitions {
			t := &transitions[i]
			if err := q.ApplyTaskTransition(ctx, sqlcgen.ApplyTaskTransitionParams{
				State:         string(t.State),
				Attempt:       int64(t.Attempt),
				DepsRemaining: int64(t.DepsRemaining),
				Error:         t.Error,
				Stale:         boolToInt(t.Stale),
				StartedAt:     toUnixPtr(t.StartedAt),
				FinishedAt:    toUnixPtr(t.FinishedAt),
				NotBefore:     toUnixPtr(t.NotBefore),
				UpdatedAt:     toUnix(t.UpdatedAt),
				ID:            string(t.ID),
			}); err != nil {
				return fmt.Errorf("apply transition %s: %w", t.ID, err)
			}
		}
		return nil
	})
}

// A video's tasks and edges are deleted as part of DeleteVideo's transaction
// rather than through a port of their own: a graph without its video is not a
// state anything wants, so there is no caller for the narrower operation.

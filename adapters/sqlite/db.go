// Package sqlite is the durable implementation of every repository port.
//
// Two connection pools sit behind one type: a multi-connection read pool and a
// single-connection write pool fed by one writer goroutine. Serialising writes
// through a channel eliminates SQLite's write-lock contention instead of
// retrying it (§8.3), and every statement is prepared once at open and reused.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go driver: cgo-free is what makes the static binary work

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrClosed is returned when a write is submitted after the store has stopped.
var ErrClosed = errors.New("sqlite store is closed")

// Options are the bootstrap knobs. Everything else is a settings row (§3).
type Options struct {
	// Path is the database file, or ":memory:" for tests.
	Path string
	// ReadPoolSize defaults to GOMAXPROCS.
	ReadPoolSize int
	// BusyTimeout is passed to SQLite explicitly rather than left to chance.
	BusyTimeout time.Duration
}

func (o Options) withDefaults() Options {
	if o.ReadPoolSize <= 0 {
		o.ReadPoolSize = runtime.GOMAXPROCS(0)
	}
	if o.BusyTimeout <= 0 {
		o.BusyTimeout = 5 * time.Second
	}
	return o
}

type writeOp struct {
	ctx   context.Context
	inTx  bool
	fn    func(context.Context, *sqlcgen.Queries) error
	reply chan error
}

// Store is the SQLite-backed implementation of the repository ports.
type Store struct {
	read  *sql.DB
	write *sql.DB
	rq    *sqlcgen.Queries
	wq    *sqlcgen.Queries
	ops   chan writeOp
	log   *slog.Logger
	// closed is set once the writer goroutine has stopped, so late writes fail
	// loudly instead of blocking forever.
	closed chan struct{}
}

// Open connects both pools, runs migrations and prepares every statement.
func Open(ctx context.Context, opts Options, log *slog.Logger) (*Store, error) {
	opts = opts.withDefaults()

	// A first run points at a directory that does not exist yet.
	if dir := filepath.Dir(opts.Path); opts.Path != ":memory:" && dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %s: %w", dir, err)
		}
	}

	readDSN, writeDSN := dsn(opts, true), dsn(opts, false)

	write, err := sql.Open("sqlite", writeDSN)
	if err != nil {
		return nil, fmt.Errorf("open write pool: %w", err)
	}
	// One connection is the whole point: a single writer, no contention.
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)
	write.SetConnMaxLifetime(0)
	if err := write.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("ping write pool: %w", err), write.Close())
	}

	if err := migrate(ctx, write, log); err != nil {
		return nil, errors.Join(err, write.Close())
	}

	read, err := sql.Open("sqlite", readDSN)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open read pool: %w", err), write.Close())
	}
	read.SetMaxOpenConns(opts.ReadPoolSize)
	read.SetMaxIdleConns(opts.ReadPoolSize)
	read.SetConnMaxLifetime(0)
	if err := read.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("ping read pool: %w", err), read.Close(), write.Close())
	}

	rq, err := sqlcgen.Prepare(ctx, read)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("prepare read statements: %w", err), read.Close(), write.Close())
	}
	wq, err := sqlcgen.Prepare(ctx, write)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("prepare write statements: %w", err), read.Close(), write.Close())
	}

	return &Store{
		read:   read,
		write:  write,
		rq:     rq,
		wq:     wq,
		ops:    make(chan writeOp),
		log:    log,
		closed: make(chan struct{}),
	}, nil
}

// dsn builds a connection string with the pragmas §8.3 requires stated
// explicitly rather than inherited from defaults.
func dsn(opts Options, readOnly bool) string {
	path := opts.Path
	if path == "" {
		path = "yt-studio.db"
	}
	if path != ":memory:" && !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", opts.BusyTimeout.Milliseconds()))
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_time_format", "sqlite")
	if path == ":memory:" {
		// A shared cache keeps both pools looking at the same in-memory database.
		return "file::memory:?cache=shared&" + q.Encode()
	}
	if readOnly {
		q.Add("_txlock", "deferred")
	} else {
		q.Add("_txlock", "immediate")
	}
	return "file:" + path + "?" + q.Encode()
}

// goose configures itself through package-level globals, so setup happens once
// and the migration itself is serialised. Two stores opening concurrently — the
// normal case in tests — would otherwise race inside the library.
var (
	gooseOnce sync.Once
	gooseErr  error
	migrateMu sync.Mutex
)

func migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	migrateMu.Lock()
	defer migrateMu.Unlock()

	gooseOnce.Do(func() {
		goose.SetBaseFS(migrationsFS)
		goose.SetLogger(goose.NopLogger())
		gooseErr = goose.SetDialect("sqlite3")
	})
	if gooseErr != nil {
		return fmt.Errorf("set goose dialect: %w", gooseErr)
	}

	start := time.Now()
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	log.Debug("migrations applied", slog.Duration("took", time.Since(start)))
	return nil
}

// Run owns the single writer goroutine. It returns when ctx is done, after
// which every queued write has been answered.
func (s *Store) Run(ctx context.Context) error {
	defer close(s.closed)
	for {
		select {
		case <-ctx.Done():
			// Answer anything already queued so callers are not left blocked.
			for {
				select {
				case op := <-s.ops:
					op.reply <- ErrClosed
				default:
					return nil
				}
			}
		case op := <-s.ops:
			op.reply <- s.exec(op)
		}
	}
}

func (s *Store) exec(op writeOp) error {
	if !op.inTx {
		return op.fn(op.ctx, s.wq)
	}
	tx, err := s.write.BeginTx(op.ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := op.fn(op.ctx, s.wq.WithTx(tx)); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Store) submit(ctx context.Context, inTx bool, fn func(context.Context, *sqlcgen.Queries) error) error {
	op := writeOp{ctx: ctx, inTx: inTx, fn: fn, reply: make(chan error, 1)}
	select {
	case s.ops <- op:
	case <-s.closed:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
	// Past this point the writer owns the operation and may be reading memory
	// the caller passed in, so the caller must not walk away — not even on
	// cancellation. Every queued op is answered: Run replies to what is still
	// in the channel when it stops, and an op already executing always
	// completes. The op's own context is what bounds how long that takes.
	return <-op.reply
}

// do runs one statement on the writer goroutine.
func (s *Store) do(ctx context.Context, fn func(context.Context, *sqlcgen.Queries) error) error {
	return s.submit(ctx, false, fn)
}

// doTx runs several statements as one transaction on the writer goroutine.
// N task transitions committing together is exactly this (§8.3).
func (s *Store) doTx(ctx context.Context, fn func(context.Context, *sqlcgen.Queries) error) error {
	return s.submit(ctx, true, fn)
}

// Close releases both pools. Call it after Run has returned.
func (s *Store) Close() error {
	return errors.Join(s.rq.Close(), s.wq.Close(), s.read.Close(), s.write.Close())
}

// ReadDB exposes the read pool for diagnostics such as EXPLAIN QUERY PLAN
// assertions in tests. Nothing in the application uses it.
func (s *Store) ReadDB() *sql.DB { return s.read }

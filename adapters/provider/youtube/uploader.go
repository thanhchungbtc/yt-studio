package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// YouTube's limits on a listing. Exceeding one is a 400 on the request that
// opens the session — or worse, on the one that closes it — so they are applied
// here, before a byte of a render goes up.
// statusResumeIncomplete is what a resumable upload answers with while it is
// still hungry. Not in net/http: 308 is Permanent Redirect there, and this is
// the other thing the number has come to mean.
const statusResumeIncomplete = 308

const (
	maxTitleRunes       = 100
	maxDescriptionRunes = 5000
	// The tag list is bounded in total, not per tag.
	maxTagsBytes = 500
)

// defaultCategoryID is Entertainment, and defaultPrivacy is the only safe
// default there is: an upload that turns out wrong is a video the operator can
// delete quietly rather than one the subscribers already saw.
const (
	defaultCategoryID = "24"
	defaultPrivacy    = "private"
)

// videoResource is the part of a video we send and the part we read back.
type videoResource struct {
	ID      string       `json:"id,omitempty"`
	Snippet videoSnippet `json:"snippet"`
	Status  videoStatus  `json:"status"`
}

type videoSnippet struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	CategoryID  string   `json:"categoryId"`
}

type videoStatus struct {
	PrivacyStatus string `json:"privacyStatus"`
	// SelfDeclaredMadeForKids is a declaration YouTube requires of every upload
	// and will not infer. Left unsent, the video lands needing attention before
	// it can be seen at all. False is the answer for long-form narration aimed
	// at adults, which is what this program makes; a channel that makes
	// children's content has one setting to change and should change it here.
	SelfDeclaredMadeForKids bool `json:"selfDeclaredMadeForKids"`
}

// Upload publishes one finished render and returns the receipt.
//
// The order is deliberate. Everything that can be checked without spending
// bandwidth is checked first — the render exists, the thumbnail exists, the
// listing is within YouTube's limits, the grant still refreshes — because each
// of those failing after four hundred megabytes have gone up is the same
// failure discovered at the worst possible moment.
func (c *Client) Upload(ctx context.Context, req provider.UploadRequest) (entity.UploadRecord, error) {
	info, err := c.store.Stat(ctx, req.FinalAssetID, entity.AssetKindFinal)
	if err != nil {
		return entity.UploadRecord{}, fmt.Errorf("stat final render: %w", err)
	}
	if info.Size == 0 {
		return entity.UploadRecord{}, fmt.Errorf("final render %s is empty", req.FinalAssetID.Short())
	}
	// Statted rather than opened: it is sent in a second request after the video
	// exists, and a thumbnail that has gone missing should stop this before the
	// upload rather than after it.
	if req.ThumbnailAssetID != "" {
		if _, err := c.store.Stat(ctx, req.ThumbnailAssetID, entity.AssetKindThumbnail); err != nil {
			return entity.UploadRecord{}, fmt.Errorf("stat thumbnail: %w", err)
		}
	}

	body := c.listing(req)

	// Last, because it is the only check that costs a round trip, and because a
	// dry run wants it too: proving the grant still refreshes is most of what
	// there is to rehearse.
	bearer, err := c.bearer(ctx, req.ChannelSlug)
	if err != nil {
		return entity.UploadRecord{}, err
	}

	if req.DryRun {
		c.log.Info("youtube dry run: everything checked, nothing sent",
			slog.String("channel", string(req.ChannelSlug)),
			slog.String("video", string(req.VideoRef)),
			slog.String("title", body.Snippet.Title),
			slog.String("privacy", body.Status.PrivacyStatus),
			slog.Int64("bytes", info.Size))
		// The render is not read. The sample backend reads it to make its
		// progress bar a measurement, which it must, because a simulated uplink
		// is all it has. This one has a real uplink and is being asked not to
		// use it, and reading four hundred megabytes to discard them would
		// report progress against work nobody wanted done.
		return entity.UploadRecord{
			VideoID:    "dry-run",
			URL:        watchPrefix + "dry-run",
			DryRun:     true,
			UploadedAt: time.Now().UTC(),
		}, nil
	}

	session, err := c.openSession(ctx, bearer, body, info.Size)
	if err != nil {
		return entity.UploadRecord{}, err
	}

	videoID, err := c.send(ctx, bearer, session, req, info.Size)
	if err != nil {
		return entity.UploadRecord{}, err
	}

	// Past this line nothing may return an error. The video is on YouTube, and
	// the receipt is written by the caller from what we return — so a failure
	// reported now loses the id of a video that exists, and the retry that
	// follows uploads a second copy of it. A wrong thumbnail is one cheap call
	// to put right; a duplicate is not.
	url := watchPrefix + videoID
	if req.ThumbnailAssetID != "" {
		if err := c.setThumbnail(ctx, bearer, videoID, req.ThumbnailAssetID); err != nil {
			c.log.Warn("youtube thumbnail was not set; the video is published with a chosen frame",
				slog.String("video", string(req.VideoRef)),
				slog.String("url", url),
				slog.String("error", err.Error()))
		}
	}

	c.log.Info("youtube upload complete",
		slog.String("channel", string(req.ChannelSlug)),
		slog.String("video", string(req.VideoRef)),
		slog.String("url", url),
		slog.String("privacy", body.Status.PrivacyStatus))

	return entity.UploadRecord{
		VideoID:    videoID,
		URL:        url,
		DryRun:     false,
		UploadedAt: time.Now().UTC(),
	}, nil
}

// listing builds the video resource, bounded to what YouTube accepts.
//
// Trimmed rather than refused. The alternative is a finished render that cannot
// be published because a generated title came out three characters long, and a
// title cut at a hundred runes is a thing an operator can see and fix on the
// listing where a blocked pipeline is not.
func (c *Client) listing(req provider.UploadRequest) videoResource {
	title := clip(sanitize(req.Metadata.Title), maxTitleRunes)
	if title == "" {
		// YouTube refuses an empty title outright, and the ref is at least a
		// true thing to call it.
		title = string(req.VideoRef)
	}
	if title != req.Metadata.Title {
		c.log.Warn("youtube title was adjusted to fit",
			slog.String("video", string(req.VideoRef)),
			slog.Int("was", len([]rune(req.Metadata.Title))),
			slog.Int("now", len([]rune(title))))
	}

	description := clip(sanitize(req.Metadata.Description), maxDescriptionRunes)

	category := strings.TrimSpace(req.Metadata.CategoryID)
	if category == "" {
		category = defaultCategoryID
	}
	privacy := strings.ToLower(strings.TrimSpace(req.Metadata.Privacy))
	switch privacy {
	case "private", "unlisted", "public":
	default:
		// Including the empty case. An unrecognised privacy is not a reason to
		// guess upward.
		privacy = defaultPrivacy
	}

	return videoResource{
		Snippet: videoSnippet{
			Title:       title,
			Description: description,
			Tags:        boundTags(req.Metadata.Tags),
			CategoryID:  category,
		},
		Status: videoStatus{PrivacyStatus: privacy, SelfDeclaredMadeForKids: false},
	}
}

// sanitize removes the two characters YouTube rejects a title or description
// for containing outright.
func sanitize(s string) string {
	return strings.TrimSpace(strings.NewReplacer("<", "", ">", "").Replace(s))
}

// clip cuts to a rune count, so a multi-byte character is never halved.
func clip(s string, runes int) string {
	r := []rune(s)
	if len(r) <= runes {
		return s
	}
	return strings.TrimSpace(string(r[:runes]))
}

// boundTags drops tags from the end until the list fits. Dropping from the end
// keeps the ones the generator thought most relevant, which is the order it
// writes them in.
func boundTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	total := 0
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		// Each tag costs its own length; a tag containing a space is quoted by
		// YouTube and costs two more.
		cost := len(tag)
		if strings.Contains(tag, " ") {
			cost += 2
		}
		if total+cost > maxTagsBytes {
			break
		}
		total += cost
		out = append(out, tag)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// openSession asks YouTube where to put the bytes.
//
// The session URI it answers with is what makes the upload resumable, and is
// good for days: a transfer interrupted halfway can be continued against it
// rather than restarted. Nothing persists it yet, so a restart mid-upload does
// start over — but the protocol's half of that is already here.
func (c *Client) openSession(ctx context.Context, bearer string, body videoResource, size int64) (string, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode video resource: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()

	endpoint := videosEndpoint + "?uploadType=resumable&part=snippet,status"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("build upload session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", entity.AssetKindFinal.MIME())
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("open youtube upload session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", apiError("open youtube upload session", resp.StatusCode, payload)
	}
	session := resp.Header.Get("Location")
	if session == "" {
		return "", errors.New("youtube accepted the upload session but returned no location")
	}
	return session, nil
}

// send pushes the render up the session in chunks, reporting as it goes, and
// returns the id of the video that results.
func (c *Client) send(
	ctx context.Context,
	bearer, session string,
	req provider.UploadRequest,
	size int64,
) (string, error) {
	file, err := c.store.Open(ctx, req.FinalAssetID, entity.AssetKindFinal)
	if err != nil {
		return "", fmt.Errorf("open final render: %w", err)
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, chunkSize)
	var sent int64
	for sent < size {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := io.ReadFull(file, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read final render: %w", err)
		}
		if n == 0 {
			return "", fmt.Errorf("final render ended after %d of %d bytes", sent, size)
		}

		id, accepted, err := c.sendChunk(ctx, bearer, session, buf[:n], sent, size)
		if err != nil {
			return "", err
		}
		if id != "" {
			if req.OnPercent != nil {
				req.OnPercent(100)
			}
			return id, nil
		}

		// What YouTube says it has, not what we say we sent. A chunk can be
		// accepted in part, and continuing from our own count would leave a hole
		// in the middle of somebody's video.
		if accepted != sent+int64(n) {
			if _, err := file.Seek(accepted, io.SeekStart); err != nil {
				return "", fmt.Errorf("rewind final render to %d: %w", accepted, err)
			}
		}
		sent = accepted
		if req.OnPercent != nil {
			req.OnPercent(int(sent * 100 / size))
		}
	}

	// Every byte is up and no response carried an id, which the protocol does
	// not allow. Reported rather than papered over: the alternative is claiming
	// a publish nobody can find.
	return "", errors.New("youtube took the whole render without returning a video id")
}

// sendChunk puts one chunk and reports what came of it: a video id when that
// chunk was the last, otherwise how far YouTube has got.
func (c *Client) sendChunk(
	ctx context.Context,
	bearer, session string,
	chunk []byte,
	offset, size int64,
) (videoID string, accepted int64, err error) {
	last := offset + int64(len(chunk))
	contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, last-1, size)

	var failure error
	for attempt := 1; attempt <= chunkAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		if attempt > 1 {
			// Where the chunk resumes from is asked rather than assumed: the
			// failed attempt may have been accepted in full and lost on the way
			// back, in which case re-sending it is both wrong and unnecessary.
			at, done, id, queryErr := c.sessionOffset(ctx, bearer, session, size)
			if queryErr != nil {
				failure = queryErr
				continue
			}
			if done {
				return id, size, nil
			}
			if at != offset {
				return "", at, nil
			}
		}

		id, at, retryable, err := c.putChunk(ctx, bearer, session, chunk, contentRange, offset)
		if err == nil {
			return id, at, nil
		}
		failure = err
		if !retryable {
			return "", 0, err
		}
		c.log.Warn("youtube chunk failed; retrying",
			slog.Int("attempt", attempt),
			slog.Int64("offset", offset),
			slog.String("error", err.Error()))
		select {
		case <-ctx.Done():
			return "", 0, ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return "", 0, fmt.Errorf("youtube refused the chunk at %d after %d attempts: %w",
		offset, chunkAttempts, failure)
}

// putChunk is one attempt at one chunk.
func (c *Client) putChunk(
	ctx context.Context,
	bearer, session string,
	chunk []byte,
	contentRange string,
	offset int64,
) (videoID string, accepted int64, retryable bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, chunkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, session, bytes.NewReader(chunk))
	if err != nil {
		return "", 0, false, fmt.Errorf("build chunk request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", entity.AssetKindFinal.MIME())
	req.Header.Set("Content-Range", contentRange)
	req.ContentLength = int64(len(chunk))

	resp, err := c.http.Do(req)
	if err != nil {
		// A dropped connection is the failure a resumable upload exists for.
		return "", 0, true, fmt.Errorf("send chunk at %d: %w", offset, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		var out videoResource
		if err := json.Unmarshal(body, &out); err != nil {
			return "", 0, false, fmt.Errorf("decode published video: %w", err)
		}
		if out.ID == "" {
			return "", 0, false, errors.New("youtube finished the upload without a video id")
		}
		return out.ID, offset + int64(len(chunk)), false, nil

	case resp.StatusCode == statusResumeIncomplete:
		at, err := rangeEnd(resp.Header.Get("Range"))
		if err != nil {
			return "", 0, false, err
		}
		return "", at, false, nil

	case resp.StatusCode == http.StatusRequestTimeout ||
		resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= 500:
		return "", 0, true, apiError("send chunk", resp.StatusCode, body)

	default:
		// A 400 or a 403 here is a rejected upload, not a bad connection.
		return "", 0, false, apiError("send chunk", resp.StatusCode, body)
	}
}

// sessionOffset asks how much of the render YouTube has, which is the only way
// to resume honestly after a failure.
func (c *Client) sessionOffset(
	ctx context.Context,
	bearer, session string,
	size int64,
) (accepted int64, done bool, videoID string, err error) {
	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, session, http.NoBody)
	if err != nil {
		return 0, false, "", fmt.Errorf("build session query: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
	req.ContentLength = 0

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, false, "", fmt.Errorf("query upload session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		// It finished after all, and the response that said so was the one that
		// went missing.
		var out videoResource
		if err := json.Unmarshal(body, &out); err != nil || out.ID == "" {
			return 0, false, "", errors.New("youtube reports the upload complete but named no video")
		}
		return size, true, out.ID, nil
	case statusResumeIncomplete:
		header := resp.Header.Get("Range")
		if header == "" {
			// No range at all means nothing has been stored yet.
			return 0, false, "", nil
		}
		at, err := rangeEnd(header)
		return at, false, "", err
	default:
		return 0, false, "", apiError("query upload session", resp.StatusCode, body)
	}
}

// rangeEnd reads the byte after the last one YouTube has stored, out of the
// `bytes=0-8388607` it reports progress with.
func rangeEnd(header string) (int64, error) {
	_, after, found := strings.Cut(header, "-")
	if !found {
		return 0, fmt.Errorf("youtube reported an unreadable range %q", header)
	}
	last, err := strconv.ParseInt(strings.TrimSpace(after), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("youtube reported an unreadable range %q", header)
	}
	return last + 1, nil
}

// setThumbnail replaces the frame YouTube would otherwise choose.
//
// A second call by YouTube's design: a thumbnail cannot be sent with the video,
// only set on one that exists. Small enough to go up in one request.
func (c *Client) setThumbnail(ctx context.Context, bearer, videoID string, asset entity.AssetID) error {
	file, err := c.store.Open(ctx, asset, entity.AssetKindThumbnail)
	if err != nil {
		return fmt.Errorf("open thumbnail: %w", err)
	}
	defer func() { _ = file.Close() }()
	image, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read thumbnail: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, chunkTimeout)
	defer cancel()

	endpoint := thumbsEndpoint + "?uploadType=media&videoId=" + videoID
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(image))
	if err != nil {
		return fmt.Errorf("build thumbnail request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", entity.AssetKindThumbnail.MIME())
	req.ContentLength = int64(len(image))

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("set thumbnail: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return apiError("set thumbnail", resp.StatusCode, body)
	}
	return nil
}

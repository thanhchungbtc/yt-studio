---
name: brainstorm
description: Brainstorm the next video for a yt-studio channel. Reads past videos and their chapter titles from ~/.yt-studio, proposes concepts that do not repeat covered ground, and emits JSON ready to POST to /api/videos. Use when asked for video ideas, what to make next, or to plan a new video.
---

# Brainstorm the next video

## 1. Read what the channel has already made

```sh
DB=~/.yt-studio/db/yt-studio.db

sqlite3 "$DB" "select slug, name, description from channels;"

sqlite3 "$DB" "
  select v.ref, v.state, v.chapter_count, v.target_duration_minutes, v.title, v.topic
  from videos v order by v.created_at;"

sqlite3 "$DB" "
  select v.ref, c.ordinal, c.title
  from chapters c join videos v on v.id = c.video_id
  order by v.created_at, c.ordinal;"
```

Read all of it — the whole history is about 10 KB. Failed and cancelled videos
count as covered ground: the topic was spent even if the render died.

## 2. What this channel is

Sleepy Mind Lab: long-form late-night listening. Calm, curious, intellectually
rich, broadly accessible — ideas that quietly shift how the listener sees
reality, themselves, existence. Across philosophy, psychology, science,
history, thought experiments, biology, cosmology, physics.

The format is settled and you should not fight it:

- `[N] [ideas | mysteries | experiments | questions] about [the mind | human
  nature | reality]`, often with `— 3 Hours of …` appended
- 180 minutes, 50–80 chapters, one orthogonal idea per chapter
- narration runs ~130 words/minute

The full voice and the chapter rules live in
`adapters/provider/ninerouter/prompts/blueprint.system.tmpl`. Read it before
proposing anything.

## 3. Propose

Offer 6–8 concepts. For each, one line of what it is, plus an honest overlap
check against every chapter title already published:

```
50 Experiments That Changed What We Know About People   ✓ clear
60 Questions Philosophy Never Answered                  ⚠ 4 overlap with SML-6
```

Overlap means the same *mechanism*, not the same words. "Ship of Theseus" and
"every cell in your body is replaced" are the same chapter under two titles.

Then stop and let the operator cut. Do not pick for them.

## 4. Emit JSON

Once one is chosen, settle the chapter count and duration, then emit exactly
this and nothing else:

```json
{
  "channel": "sleepy-mind-lab",
  "title": "50 Experiments That Changed What We Know About People",
  "topic": "A brief, not a caption. What the video walks through, in the channel's own register — concrete named territory, so the blueprint knows what ground to cover. End with a line listing the concepts already taught in earlier videos that this one must not re-derive.",
  "chapterCount": 50,
  "slidesPerChapter": 1,
  "thumbnailCells": 6,
  "targetDurationMinutes": 180,
  "start": false
}
```

`topic` caps at 5000 characters. The "already covered, do not repeat" line at
the end matters: the blueprint call never sees past videos, so this is the only
place cross-video dedupe can be stated.

Write the JSON to a file and tell the operator both ways to use it:

```sh
curl -X POST localhost:8080/api/videos -H 'Content-Type: application/json' -d @idea.json
```

or paste the fields into the New Video dialog.

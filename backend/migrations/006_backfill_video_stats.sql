-- Backfill video_stats from actual likes/comments data.
-- Backup already created in 005 before comments.id type change.
-- Requires old writes to be stopped and legacy queues drained before running.
INSERT INTO video_stats (video_id, like_count, comment_count, stats_version, updated_at)
SELECT
  v.id AS video_id,
  COALESCE(l.like_count, 0) AS like_count,
  COALESCE(c.comment_count, 0) AS comment_count,
  1 AS stats_version,
  NOW(3) AS updated_at
FROM videos v
LEFT JOIN (
  SELECT video_id, COUNT(*) AS like_count
  FROM likes
  WHERE deleted_at IS NULL
  GROUP BY video_id
) l ON l.video_id = v.id
LEFT JOIN (
  SELECT video_id, COUNT(*) AS comment_count
  FROM comments
  WHERE deleted_at IS NULL
  GROUP BY video_id
) c ON c.video_id = v.id
ON DUPLICATE KEY UPDATE
  like_count = VALUES(like_count),
  comment_count = VALUES(comment_count),
  stats_version = GREATEST(video_stats.stats_version, VALUES(stats_version)),
  updated_at = VALUES(updated_at);

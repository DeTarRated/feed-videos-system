ALTER TABLE likes
  ADD COLUMN op_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD KEY idx_likes_video_version (video_id, op_version);

CREATE TABLE IF NOT EXISTS comments_backup_005 AS
SELECT * FROM comments;

ALTER TABLE comments
  ADD COLUMN op_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN operation_id VARCHAR(64) NULL,
  ADD COLUMN request_fingerprint VARCHAR(128) NOT NULL DEFAULT '',
  ADD UNIQUE KEY uk_comments_operation (operation_id),
  ADD KEY idx_comments_id_fingerprint (id, request_fingerprint);

ALTER TABLE comments
  MODIFY COLUMN id VARCHAR(36) NOT NULL;

CREATE TABLE IF NOT EXISTS video_stats (
  video_id BIGINT UNSIGNED NOT NULL,
  like_count BIGINT NOT NULL DEFAULT 0,
  comment_count BIGINT NOT NULL DEFAULT 0,
  stats_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (video_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS interaction_outcomes (
  operation_id VARCHAR(64) NOT NULL,
  request_fingerprint VARCHAR(128) NOT NULL,
  command_type VARCHAR(32) NOT NULL,
  business_key VARCHAR(128) NOT NULL,
  op_version BIGINT UNSIGNED NOT NULL,
  video_id BIGINT UNSIGNED NOT NULL,
  db_outcome VARCHAR(32) NOT NULL,
  result_state VARCHAR(64) NOT NULL,
  like_count BIGINT NOT NULL DEFAULT 0,
  comment_count BIGINT NOT NULL DEFAULT 0,
  stats_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
  heat_delta BIGINT NOT NULL DEFAULT 0,
  heat_minute VARCHAR(16) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (operation_id),
  KEY idx_outcomes_heat_minute (heat_minute),
  KEY idx_outcomes_business_key (business_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

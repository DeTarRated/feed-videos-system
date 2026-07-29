CREATE TABLE IF NOT EXISTS follows (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  follower_id BIGINT UNSIGNED NOT NULL,
  following_id BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_follows_follower_following (follower_id, following_id),
  KEY idx_follows_follower_deleted (follower_id, deleted_at),
  KEY idx_follows_following_deleted (following_id, deleted_at),
  CONSTRAINT fk_follows_follower FOREIGN KEY (follower_id) REFERENCES users(id),
  CONSTRAINT fk_follows_following FOREIGN KEY (following_id) REFERENCES users(id),
  CONSTRAINT chk_follows_not_self CHECK (follower_id <> following_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

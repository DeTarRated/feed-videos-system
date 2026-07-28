CREATE TABLE IF NOT EXISTS likes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  video_id BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_likes_user_video (user_id, video_id),
  KEY idx_likes_video_deleted (video_id, deleted_at),
  KEY idx_likes_user_deleted (user_id, deleted_at),
  CONSTRAINT fk_likes_user FOREIGN KEY (user_id) REFERENCES users(id),
  CONSTRAINT fk_likes_video FOREIGN KEY (video_id) REFERENCES videos(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

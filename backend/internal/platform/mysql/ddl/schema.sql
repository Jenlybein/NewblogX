-- 本文件由 cmd/cli 的一次性生成器从 Go 行结构导出，**请勿手工编辑**。
--
-- 为什么建库要用 SQL 而不是运行时的 AutoMigrate：
--   1. 零列变更口径下，表结构是**冻结的契约**，冻结的东西就该存成文本、进版本库、
--      可 review；每次运行时从 Go 结构推导，等于让「当前代码长什么样」决定「库长什么样」。
--   2. M6 删掉 models 之后，Go 侧不再有任何地方持有「全表」这一概念，
--      AutoMigrate 无处可依。
--
-- 生成来源：当前 models 的全部行结构（AutoMigrate 后 SHOW CREATE TABLE）。
-- 一致性保障：internal/platform/mysql/schema_contract_integration_test.go 会把本文件
-- 应用到一个空库，再与各模块自持的行结构逐列比对。

CREATE TABLE IF NOT EXISTS `ai_provider_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `name` varchar(64) NOT NULL,
  `vendor` varchar(32) NOT NULL,
  `model` varchar(128) NOT NULL,
  `enabled` tinyint(1) DEFAULT NULL,
  `config` longtext NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ai_provider_models_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_ai_score_record_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `article_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `title_snapshot` varchar(256) NOT NULL,
  `content_hash` varchar(64) NOT NULL,
  `content_length` bigint DEFAULT '0',
  `article_updated_at_snapshot` datetime(3) DEFAULT NULL,
  `ai_total_score` bigint DEFAULT '0',
  `total_score` bigint DEFAULT '0',
  `score_level` varchar(32) DEFAULT NULL,
  `article_type` varchar(32) DEFAULT NULL,
  `dimensions_json` longtext NOT NULL,
  `main_issues_json` longtext NOT NULL,
  `overall_comment` longtext,
  `provider` varchar(32) DEFAULT NULL,
  `model_name` varchar(64) DEFAULT NULL,
  `prompt_version` varchar(32) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_article_ai_score_record_models_article_id` (`article_id`),
  KEY `idx_article_ai_score_record_models_user_id` (`user_id`),
  KEY `idx_article_ai_score_record_models_content_hash` (`content_hash`),
  KEY `idx_article_ai_score_record_models_total_score` (`total_score`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_digg_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `article_id` bigint unsigned DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_article_digg` (`article_id`,`user_id`),
  KEY `fk_article_digg_models_user_model` (`user_id`),
  CONSTRAINT `fk_article_digg_models_article_model` FOREIGN KEY (`article_id`) REFERENCES `article_models` (`id`),
  CONSTRAINT `fk_article_digg_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_hot_top_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `article_id` bigint unsigned NOT NULL,
  `sort_order` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_article_hot_top` (`article_id`),
  CONSTRAINT `fk_article_hot_top_models_article_model` FOREIGN KEY (`article_id`) REFERENCES `article_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_message_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `type` tinyint DEFAULT NULL,
  `receiver_id` bigint unsigned DEFAULT NULL,
  `action_user_id` bigint unsigned DEFAULT NULL,
  `action_user_nickname` longtext,
  `action_user_avatar` longtext,
  `content` text,
  `article_id` bigint unsigned DEFAULT NULL,
  `comment_id` bigint unsigned DEFAULT NULL,
  `article_title` longtext,
  `link_title` varchar(128) DEFAULT NULL,
  `link_herf` varchar(256) DEFAULT NULL,
  `is_read` tinyint(1) DEFAULT '0',
  `read_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_msg_receiver_read_created` (`receiver_id`,`is_read`),
  KEY `idx_article_message_models_action_user_id` (`action_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `title` varchar(256) DEFAULT NULL,
  `abstract` varchar(256) DEFAULT NULL,
  `content` longtext,
  `category_id` bigint unsigned DEFAULT NULL,
  `cover` varchar(256) DEFAULT NULL,
  `author_id` bigint unsigned DEFAULT NULL,
  `view_count` bigint DEFAULT '0',
  `digg_count` bigint DEFAULT '0',
  `comment_count` bigint DEFAULT '0',
  `favor_count` bigint DEFAULT '0',
  `comments_toggle` tinyint(1) DEFAULT '1',
  `publish_status` tinyint DEFAULT '0',
  `visibility_status` varchar(32) DEFAULT 'visible',
  `submitted_at` datetime(3) DEFAULT NULL,
  `reviewed_at` datetime(3) DEFAULT NULL,
  `reviewed_by` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_article_models_category_id` (`category_id`),
  KEY `idx_article_models_author_id` (`author_id`),
  KEY `idx_article_models_publish_status` (`publish_status`),
  KEY `idx_article_models_visibility_status` (`visibility_status`),
  KEY `idx_article_models_reviewed_by` (`reviewed_by`),
  CONSTRAINT `fk_article_models_user_model` FOREIGN KEY (`author_id`) REFERENCES `user_models` (`id`),
  CONSTRAINT `fk_category_models_article_list` FOREIGN KEY (`category_id`) REFERENCES `category_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_review_log_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `task_id` bigint unsigned NOT NULL,
  `article_id` bigint unsigned NOT NULL,
  `operator_id` bigint unsigned NOT NULL,
  `action` varchar(32) NOT NULL,
  `from_status` varchar(32) DEFAULT NULL,
  `to_status` varchar(32) DEFAULT NULL,
  `reason` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_article_review_log_models_task_id` (`task_id`),
  KEY `idx_article_review_log_models_article_id` (`article_id`),
  KEY `idx_article_review_log_models_operator_id` (`operator_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_review_task_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `article_id` bigint unsigned NOT NULL,
  `author_id` bigint unsigned NOT NULL,
  `article_title` varchar(256) NOT NULL,
  `author_name` varchar(32) NOT NULL,
  `article_publish_status` tinyint DEFAULT '0',
  `stage` varchar(16) NOT NULL DEFAULT 'manual',
  `source` varchar(16) NOT NULL,
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `reason` varchar(500) DEFAULT NULL,
  `reviewed_at` datetime(3) DEFAULT NULL,
  `reviewed_by` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_article_review_task_models_article_id` (`article_id`),
  KEY `idx_article_review_task_models_author_id` (`author_id`),
  KEY `idx_article_review_task_models_article_publish_status` (`article_publish_status`),
  KEY `idx_article_review_task_models_status` (`status`),
  KEY `idx_article_review_task_models_reviewed_by` (`reviewed_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `article_tag_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `article_id` bigint unsigned DEFAULT NULL,
  `tag_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_article_tag` (`article_id`,`tag_id`),
  KEY `idx_article_tag_article` (`article_id`),
  KEY `idx_article_tag_tag` (`tag_id`),
  CONSTRAINT `fk_article_tag_models_article` FOREIGN KEY (`article_id`) REFERENCES `article_models` (`id`),
  CONSTRAINT `fk_article_tag_models_tag` FOREIGN KEY (`tag_id`) REFERENCES `tag_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `banner_crop_models` (
  `banner_id` bigint unsigned NOT NULL,
  `focus_x` double DEFAULT NULL,
  `focus_y` double DEFAULT NULL,
  `zoom` double DEFAULT NULL,
  PRIMARY KEY (`banner_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `banner_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `show` tinyint(1) DEFAULT NULL,
  `cover` longtext,
  `href` longtext,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `category_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `title` varchar(256) DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_category_user_title` (`user_id`,`title`),
  KEY `idx_category_models_user_id` (`user_id`),
  CONSTRAINT `fk_category_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `cdc_dead_letter` (
  `id` bigint unsigned NOT NULL,
  `stream` varchar(32) NOT NULL,
  `cdc_job_id` varchar(255) NOT NULL,
  `source_table` varchar(128) NOT NULL,
  `action` varchar(32) NOT NULL,
  `target_key` varchar(255) DEFAULT NULL,
  `payload_json` longtext NOT NULL,
  `retry_count` bigint NOT NULL DEFAULT '0',
  `status` varchar(32) NOT NULL,
  `error_code` varchar(64) DEFAULT NULL,
  `error_msg` text,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_cdc_dlq_stream_job` (`stream`,`cdc_job_id`),
  KEY `idx_cdc_dlq_stream_status_created` (`stream`,`status`,`created_at`),
  KEY `idx_cdc_dlq_job_id` (`cdc_job_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `chat_msg_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `sender_id` bigint unsigned DEFAULT NULL,
  `receiver_id` bigint unsigned DEFAULT NULL,
  `session_id` varchar(64) NOT NULL,
  `content` longtext,
  `send_time` datetime(3) DEFAULT NULL,
  `read_at` datetime(3) DEFAULT NULL,
  `msg_status` tinyint DEFAULT NULL,
  `msg_type` tinyint DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_chat_msg_session_time` (`session_id`,`send_time` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `chat_msg_user_state_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `msg_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `session_id` varchar(64) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_chat_msg_user_state` (`msg_id`,`user_id`),
  KEY `idx_chat_msg_user_msg` (`user_id`,`msg_id`),
  KEY `idx_chat_msg_user_session_deleted` (`user_id`,`session_id`,`msg_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `chat_session_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `session_id` varchar(64) NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `receiver_id` bigint unsigned NOT NULL,
  `receiver_nickname` varchar(64) DEFAULT NULL,
  `receiver_avatar` varchar(256) DEFAULT NULL,
  `last_msg_id` bigint unsigned DEFAULT NULL,
  `last_msg_content` longtext,
  `last_msg_time` datetime(3) DEFAULT NULL,
  `clear_before_msg_id` bigint unsigned DEFAULT NULL,
  `unread_count` bigint DEFAULT NULL,
  `is_top` tinyint(1) DEFAULT NULL,
  `is_mute` tinyint(1) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_chat_session_user_receiver` (`user_id`,`receiver_id`),
  KEY `idx_chat_session_id` (`session_id`),
  KEY `idx_chat_session_user_time` (`user_id`,`last_msg_time` DESC),
  KEY `fk_chat_session_models_receiver_model` (`receiver_id`),
  CONSTRAINT `fk_chat_session_models_receiver_model` FOREIGN KEY (`receiver_id`) REFERENCES `user_models` (`id`),
  CONSTRAINT `fk_chat_session_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `comment_digg_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `comment_id` bigint unsigned DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_comment_digg` (`comment_id`,`user_id`),
  KEY `fk_comment_digg_models_user_model` (`user_id`),
  CONSTRAINT `fk_comment_digg_models_comment_model` FOREIGN KEY (`comment_id`) REFERENCES `comment_models` (`id`),
  CONSTRAINT `fk_comment_digg_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `comment_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `content` text NOT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  `user_nickname` varchar(64) DEFAULT NULL,
  `user_avatar` varchar(256) DEFAULT NULL,
  `article_id` bigint unsigned NOT NULL,
  `reply_id` bigint unsigned DEFAULT '0',
  `reply_user_id` bigint unsigned DEFAULT '0',
  `reply_user_nickname` varchar(64) DEFAULT NULL,
  `root_id` bigint unsigned DEFAULT '0',
  `digg_count` bigint DEFAULT '0',
  `reply_count` bigint DEFAULT '0',
  `status` tinyint DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_article_root` (`article_id`,`root_id`),
  KEY `fk_comment_models_user_model` (`user_id`),
  KEY `fk_comment_models_parent_model` (`reply_id`),
  CONSTRAINT `fk_comment_models_article_model` FOREIGN KEY (`article_id`) REFERENCES `article_models` (`id`),
  CONSTRAINT `fk_comment_models_parent_model` FOREIGN KEY (`reply_id`) REFERENCES `comment_models` (`id`),
  CONSTRAINT `fk_comment_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `favorite_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  `title` varchar(32) DEFAULT NULL,
  `cover` varchar(256) DEFAULT NULL,
  `abstract` varchar(256) DEFAULT NULL,
  `is_default` tinyint(1) DEFAULT '0',
  `article_count` bigint DEFAULT '0',
  `owner_nickname` varchar(64) DEFAULT NULL,
  `owner_avatar` varchar(256) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_favorite_user_title` (`user_id`,`title`),
  KEY `idx_favorite_models_user_id` (`user_id`),
  CONSTRAINT `fk_favorite_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `global_notif_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_visible_rule` tinyint DEFAULT NULL,
  `expire_time` datetime(3) DEFAULT NULL,
  `action_user` bigint unsigned DEFAULT NULL,
  `title` varchar(64) DEFAULT NULL,
  `icon` varchar(64) DEFAULT NULL,
  `content` varchar(128) DEFAULT NULL,
  `href` varchar(256) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `image_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  `provider` varchar(16) DEFAULT 'qiniu',
  `bucket` varchar(64) DEFAULT NULL,
  `object_key` varchar(255) DEFAULT NULL,
  `file_name` varchar(255) DEFAULT NULL,
  `url` varchar(512) DEFAULT NULL,
  `mime_type` varchar(128) DEFAULT NULL,
  `size` bigint DEFAULT '0',
  `width` bigint DEFAULT '0',
  `height` bigint DEFAULT '0',
  `hash` varchar(64) DEFAULT NULL,
  `status` bigint unsigned DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_image_object_key` (`object_key`),
  UNIQUE KEY `uk_image_hash` (`hash`),
  KEY `idx_image_models_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `image_ref_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `image_id` bigint unsigned NOT NULL,
  `ref_type` bigint unsigned NOT NULL,
  `owner_id` bigint unsigned DEFAULT NULL,
  `field` bigint unsigned NOT NULL,
  `position` bigint DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_image_ref_owner_field_pos` (`ref_type`,`owner_id`,`field`,`position`,`image_id`),
  KEY `idx_image_ref_models_image_id` (`image_id`),
  KEY `idx_image_ref_models_ref_type` (`ref_type`),
  KEY `idx_image_ref_models_owner_id` (`owner_id`),
  KEY `idx_image_ref_models_field` (`field`),
  CONSTRAINT `fk_image_ref_models_image_model` FOREIGN KEY (`image_id`) REFERENCES `image_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `outbox_event_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `event_name` varchar(64) NOT NULL,
  `aggregate_type` varchar(64) NOT NULL,
  `aggregate_id` bigint unsigned NOT NULL,
  `payload` longtext,
  `status` varchar(16) NOT NULL,
  `attempts` bigint NOT NULL DEFAULT '0',
  `available_at` datetime(3) NOT NULL,
  `last_error` varchar(512) DEFAULT NULL,
  `dispatched_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_outbox_event_models_event_name` (`event_name`),
  KEY `idx_outbox_event_models_aggregate_id` (`aggregate_id`),
  KEY `idx_outbox_event_models_status` (`status`),
  KEY `idx_outbox_event_models_available_at` (`available_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 站点运行时配置：一段一行。
-- name 取值固定为 site / ai / mail / storage / oauth / log / upload。
-- 之所以不拆成"一段一列"或"一个设置一列"：本目录的 DDL 只做
-- CREATE TABLE IF NOT EXISTS，生产上没有任何 ALTER 通道，
-- 列化会让每加一个配置字段都变成一次表结构变更。
CREATE TABLE IF NOT EXISTS `runtime_config_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `name` varchar(32) NOT NULL,
  `payload` longtext NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_runtime_config_models_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `tag_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `title` varchar(64) DEFAULT NULL,
  `sort` bigint DEFAULT '0',
  `description` varchar(255) DEFAULT NULL,
  `article_count` bigint DEFAULT '0',
  `is_enabled` tinyint(1) DEFAULT '1',
  `created_by` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_tag_models_title` (`title`),
  KEY `idx_tag_models_is_enabled` (`is_enabled`),
  KEY `idx_tag_models_created_by` (`created_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_article_favor_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `article_id` bigint unsigned DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  `favor_id` bigint unsigned DEFAULT NULL,
  `article_title` varchar(256) DEFAULT NULL,
  `article_abstract` varchar(256) DEFAULT NULL,
  `article_cover` varchar(256) DEFAULT NULL,
  `article_status` tinyint DEFAULT '0',
  `article_author_id` bigint unsigned DEFAULT NULL,
  `article_author_nickname` varchar(64) DEFAULT NULL,
  `article_author_avatar` varchar(256) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_article_favor` (`article_id`,`user_id`,`favor_id`),
  KEY `idx_article_user` (`article_id`,`user_id`,`favor_id`),
  KEY `idx_user_article_favor_models_article_author_id` (`article_author_id`),
  KEY `fk_user_article_favor_models_user_model` (`user_id`),
  KEY `fk_favorite_models_article_list` (`favor_id`),
  CONSTRAINT `fk_favorite_models_article_list` FOREIGN KEY (`favor_id`) REFERENCES `favorite_models` (`id`),
  CONSTRAINT `fk_user_article_favor_models_article_model` FOREIGN KEY (`article_id`) REFERENCES `article_models` (`id`),
  CONSTRAINT `fk_user_article_favor_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_article_view_history_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `article_id` bigint unsigned DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_article_history` (`article_id`,`user_id`),
  KEY `fk_user_article_view_history_models_user_model` (`user_id`),
  CONSTRAINT `fk_user_article_view_history_models_article_model` FOREIGN KEY (`article_id`) REFERENCES `article_models` (`id`),
  CONSTRAINT `fk_user_article_view_history_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_conf_models` (
  `user_id` bigint unsigned NOT NULL,
  `like_tags` longtext,
  `updated_username_date` datetime(3) DEFAULT NULL,
  `favorites_visibility` tinyint(1) DEFAULT NULL,
  `follow_visibility` tinyint(1) DEFAULT NULL,
  `fans_visibility` tinyint(1) DEFAULT NULL,
  `home_style_id` bigint unsigned DEFAULT NULL,
  `digg_notice_enabled` tinyint(1) DEFAULT NULL,
  `comment_notice_enabled` tinyint(1) DEFAULT NULL,
  `favor_notice_enabled` tinyint(1) DEFAULT NULL,
  `private_chat_notice_enabled` tinyint(1) DEFAULT NULL,
  `stranger_chat_enabled` tinyint(1) DEFAULT NULL,
  PRIMARY KEY (`user_id`),
  CONSTRAINT `fk_user_models_user_conf_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_follow_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `followed_user_id` bigint unsigned DEFAULT NULL,
  `followed_nickname` varchar(64) DEFAULT NULL,
  `followed_avatar` varchar(256) DEFAULT NULL,
  `followed_abstract` varchar(256) DEFAULT NULL,
  `fans_user_id` bigint unsigned DEFAULT NULL,
  `fans_nickname` varchar(64) DEFAULT NULL,
  `fans_avatar` varchar(256) DEFAULT NULL,
  `fans_abstract` varchar(256) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_follow` (`followed_user_id`,`fans_user_id`),
  KEY `fk_user_follow_models_fans_user_model` (`fans_user_id`),
  CONSTRAINT `fk_user_follow_models_fans_user_model` FOREIGN KEY (`fans_user_id`) REFERENCES `user_models` (`id`),
  CONSTRAINT `fk_user_follow_models_followed_user_model` FOREIGN KEY (`followed_user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_global_notif_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `msg_id` bigint unsigned DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  `is_read` tinyint(1) DEFAULT NULL,
  `read_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_global_notif` (`msg_id`,`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `username` varchar(32) DEFAULT NULL,
  `nickname` varchar(32) DEFAULT NULL,
  `avatar` varchar(256) DEFAULT NULL,
  `abstract` varchar(256) DEFAULT NULL,
  `register_source` tinyint DEFAULT NULL,
  `password` varchar(64) DEFAULT NULL,
  `email` varchar(256) DEFAULT NULL,
  `open_id` varchar(64) DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `token_version` int unsigned DEFAULT '1',
  `last_password_changed_at` datetime(3) DEFAULT NULL,
  `role` tinyint DEFAULT '0',
  `ip` varchar(64) DEFAULT NULL,
  `addr` varchar(256) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_username` (`username`),
  UNIQUE KEY `uk_user_email` (`email`),
  UNIQUE KEY `uk_user_open_id` (`open_id`),
  KEY `idx_user_models_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_session_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` bigint unsigned NOT NULL,
  `refresh_token_hash` varchar(64) DEFAULT NULL,
  `ip` varchar(64) DEFAULT NULL,
  `addr` varchar(256) DEFAULT NULL,
  `ua` varchar(512) DEFAULT NULL,
  `last_seen_at` datetime(3) DEFAULT NULL,
  `expires_at` datetime(3) DEFAULT NULL,
  `revoked_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_session_refresh_token` (`refresh_token_hash`),
  KEY `idx_user_session_user_id` (`user_id`),
  KEY `idx_user_session_expires_at` (`expires_at`),
  KEY `idx_user_session_revoked_at` (`revoked_at`),
  CONSTRAINT `fk_user_session_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_stat_models` (
  `user_id` bigint unsigned NOT NULL,
  `view_count` bigint DEFAULT '0',
  `fans_count` bigint DEFAULT '0',
  `follow_count` bigint DEFAULT '0',
  `article_visited_count` bigint DEFAULT '0',
  `article_count` bigint DEFAULT '0',
  PRIMARY KEY (`user_id`),
  CONSTRAINT `fk_user_models_user_stat_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_top_article_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  `article_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_top_article` (`user_id`,`article_id`),
  KEY `fk_user_top_article_models_article_model` (`article_id`),
  CONSTRAINT `fk_user_top_article_models_article_model` FOREIGN KEY (`article_id`) REFERENCES `article_models` (`id`),
  CONSTRAINT `fk_user_top_article_models_user_model` FOREIGN KEY (`user_id`) REFERENCES `user_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_view_daily_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` bigint unsigned DEFAULT NULL,
  `viewer_user_id` bigint unsigned DEFAULT NULL,
  `view_date` date DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_view_daily` (`user_id`,`viewer_user_id`,`view_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;


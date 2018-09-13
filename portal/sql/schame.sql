CREATE TABLE IF NOT EXISTS sessions (
    `session_id` varchar(60) NOT NULL,
    `session_data` text,
    `created_at` int(10) unsigned NOT NULL,
    `updated_at` int(10) unsigned NOT NULL,
    PRIMARY KEY (`session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS admin_users (
    `name` varchar(64) NOT NULL,
    `password` varchar(64) NOT NULL,
    PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS admin_messages (
    `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
    `message` mediumtext,
    `type` enum('info', 'warning', 'danger') NOT NULL DEFAULT 'info',
    `disabled` tinyint(1) unsigned NOT NULL,
    `updated_at` int(10) unsigned NOT NULL,
    `created_at` int(10) unsigned NOT NULL,
    PRIMARY KEY `id`
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLe IF NOT EXISTS admin_regulations (
    `regulation` mediumtext
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS teams (
    `id` int(10) unsigned NOT NULL,
    `group_id` varchar(64) NOT NULL,
    `state` enum('actived', 'banned') DEFAULT 'actived',
    `name` varchar(256) NOT NULL,
    `password` varchar(64) NOT NULL,
    `category` enum('general_one', 'general_two', 'general_three', 'student_one', 'student_two', 'student_three') NOT NULL,
    `banned_reason` mediumtext,
    `created_at` int(10) unsigned NOT NULL,
    `updated_at` int(10) unsigned NOT NULL,
    PRIMARY KEY (`id`),
    KEY idx_group_id (`group_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS servers (
    `group_id` varchar(64) NOT NULL,
    `hostname` varchar(64) NOT NULL,
    `password` varchar(64) NOT NULL,
    `global_ip` varchar(64) NOT NULL,
    `private_ip` varchar(64) NOT NULL,
    `private_network` varchar(64) NOT NULL,
    `bench_ip` varchar(64) NOT NULL,
    `bench_network` varchar(64) NOT NULL,
    `node` varchar(64) NOT NULL,
    `is_bench_host` tinyint(1) NOT NULL,
    `is_target_host` tinyint(1) NOT NULL,
    `created_at` int(10) unsigned NOT NULL,
    `updated_at` int(10) unsigned NOT NULL,
    PRIMARY KEY (`group_id`, `global_ip`),
    KEY idx_group_id_bench_ip (`group_id`, `bench_ip`),
    KEY idx_node (`node`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS team_members (
    `team_id` int(10) unsigned NOT NULL,
    `member_number` tinyint(1) unsigned NOT NULL,
    `nickname` varchar(128) NOT NULL DEFAULT '',
    `twitter_id` varchar(128) NOT NULL DEFAULT '',
    `github_id` varchar(128) NOT NULL DEFAULT '',
    `created_at` int(10) unsigned NOT NULL,
    `updated_at` int(10) unsigned NOT NULL,
    PRIMARY KEY (`team_id`, `member_number`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS team_scores (
    `team_id` int(10) unsigned NOT NULL,
    `latest_score` bigint(20) unsigned NOT NULL,
    `best_score` bigint(20) unsigned NOT NULL,
    `latest_status` enum('pass', 'fail') NOT NULL,
    `created_at` int(10) unsigned NOT NULL,
    `updated_at` int(10) unsigned NOT NULL,
    PRIMARY KEY (`team_id`),
    KEY idx_best_score (`best_score`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS all_scores (
    `team_id` int(10) unsigned NOT NULL,
    `score` bigint(20) unsigned NOT NULL,
    `created_at` int(10) unsigned NOT NULL,
    KEY idx_team_id_and_created_at (`team_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bench_queues (
    `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
    `team_id` int(10) unsigned NOT NULL,
    `node` varchar(64) NOT NULL,
    `bench_hostname` varchar(64) NOT NULL,
    `target_ip` varchar(64) NOT NULL,
    `state` enum('waiting', 'running', 'done', 'aborted', 'canceled') NOT NULL DEFAULT 'waiting',
    `result_status` enum('pass', 'fail', 'unknown') DEFAULT 'unknown',
    `result_score` int(10) unsigned NOT NULL DEFAULT 0,
    `result_json` mediumtext,
    `log_text` mediumtext,
    `created_at` int(10) unsigned NOT NULL,
    `updated_at` int(10) unsigned NOT NULL,
    PRIMARY KEY (`id`),
    KEY idx_team_id (`team_id`),
    KEY idx_bench_hostname_and_state(`bench_hostname`),
    KEY idx_node_id_and_state (`node`, `state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


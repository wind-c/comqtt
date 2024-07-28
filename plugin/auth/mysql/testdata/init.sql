BEGIN;

CREATE TABLE auth (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(255) NOT NULL,
    allow SMALLINT DEFAULT 1 NOT NULL,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated TIMESTAMP NULL
);

CREATE TABLE acl (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    topic VARCHAR(255) NOT NULL,
    access SMALLINT DEFAULT 3 NOT NULL,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated TIMESTAMP NULL
);

CREATE INDEX acl_username_idx ON acl(username);

-- 123456
INSERT INTO auth (username, password, allow) VALUES ('zhangsan', '$2a$12$j8bs10UCRC5GUENPqZXLceACpN1l72wcDaN6F0j0rIbcHIZpt0Cbq', 1);
INSERT INTO acl (username, topic, access) VALUES ('zhangsan', 'topictest/1', 2);

COMMIT;

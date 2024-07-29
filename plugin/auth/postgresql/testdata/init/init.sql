BEGIN;
CREATE TABLE auth (
    id serial PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    allow smallint DEFAULT 1 NOT NULL,
    created timestamp with time zone DEFAULT NOW(),
    updated timestamp
);

CREATE TABLE acl(
    id serial PRIMARY KEY,
    username TEXT NOT NULL,
    topic TEXT NOT NULL,
    access smallint DEFAULT 3 NOT NULL,
    created timestamp with time zone DEFAULT NOW(),
    updated timestamp
);
CREATE INDEX acl_username_idx ON acl(username);

INSERT INTO auth (username, password, allow) VALUES ('zhangsan', '321654', 1);
INSERT INTO acl (username, topic, access) VALUES ('zhangsan', 'topictest/1', 2);

COMMIT;

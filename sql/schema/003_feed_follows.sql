-- +goose Up
CREATE TABLE IF NOT EXISTS feed_follows(
    id UUID PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_url VARCHAR(255) NOT NULL REFERENCES feeds(url) ON DELETE CASCADE,
    UNIQUE(user_id, feed_url)
);

-- +goose Down
DROP TABLE IF EXISTS feed_follows;
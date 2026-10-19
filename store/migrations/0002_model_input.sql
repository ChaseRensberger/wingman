CREATE TABLE model_input_blobs (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    hash TEXT NOT NULL,
    content BLOB NOT NULL,
    PRIMARY KEY (session_id, hash)
);

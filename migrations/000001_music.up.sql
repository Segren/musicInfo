CREATE TABLE IF NOT EXISTS songs (
    id bigserial PRIMARY KEY,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    "group" text NOT NULL,
    name text NOT NULL,
    releaseDate date,
    text text NOT NULL,
    link text,
    version integer NOT NULL DEFAULT 1
);

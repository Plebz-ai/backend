-- Ensure the database is created
CREATE DATABASE character_demo;
\c character_demo

-- Create necessary tables
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS characters (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    personality TEXT NOT NULL,
    voice_type VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id SERIAL PRIMARY KEY,
    external_id VARCHAR(255),
    character_id INTEGER REFERENCES characters(id),
    session_id VARCHAR(255),
    sender VARCHAR(50),
    content TEXT,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audio_chunks (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255),
    session_id VARCHAR(255),
    char_id INTEGER REFERENCES characters(id),
    audio_data BYTEA,
    format VARCHAR(10) DEFAULT 'webm',
    duration FLOAT,
    sample_rate INTEGER DEFAULT 48000,
    channels INTEGER DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP + INTERVAL '24 hours'),
    metadata TEXT,
    processing_status VARCHAR(20) DEFAULT 'pending'
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_messages_char_session ON messages(character_id, session_id);
CREATE INDEX IF NOT EXISTS idx_audio_session ON audio_chunks(session_id);
CREATE INDEX IF NOT EXISTS idx_audio_status ON audio_chunks(processing_status);
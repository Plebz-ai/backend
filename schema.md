# Database Schema

## User
```go
User {
  ID        uint      (Primary Key)
  Name      string
  Email     string    (Unique Index)
  Password  string    (Hashed, not exposed in JSON)
  CreatedAt time.Time
  UpdatedAt time.Time
}
```

## Character
```go
Character {
  ID          uint      (Primary Key)
  Name        string    (Not Null)
  Description string    (Not Null)
  Personality string    (Not Null)
  VoiceType   string    (Not Null)
  CreatedAt   time.Time
  UpdatedAt   time.Time
}
```

## Message
```go
Message {
  ID          uint      (Primary Key)
  ExternalID  string    (Indexed)
  CharacterID uint      (Indexed)
  SessionID   string    (Indexed)
  Sender      string
  Content     string
  Timestamp   time.Time
  CreatedAt   time.Time
}
```

## AudioChunk
```go
AudioChunk {
  ID               uint      (Primary Key)
  UserID           string
  SessionID        string    (Indexed)
  CharID           uint
  AudioData        []byte
  Format           string    (Default: "webm")
  Duration         float64   (In seconds)
  SampleRate       int       (Default: 48000)
  Channels         int       (Default: 1)
  CreatedAt        time.Time
  ExpiresAt        time.Time (Indexed, Default: 24 hours from creation)
  Metadata         string    (JSON string for additional context)
  ProcessingStatus string    (Default: "pending")
}
```

## Database Indexes
The database also includes the following indexes:
- `idx_messages_char_session` on `messages(character_id, session_id)`
- `idx_audio_session` on `audio_chunks(session_id)`
- `idx_audio_status` on `audio_chunks(processing_status)` 
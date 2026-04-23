CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid()
);

CREATE TABLE subjects (
    id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR NOT NULL UNIQUE
);

CREATE TABLE subtopics (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id UUID NOT NULL REFERENCES subjects(id),
    name       VARCHAR NOT NULL,

    UNIQUE (subject_id, name)
);

CREATE TABLE questions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subtopic_id    UUID NOT NULL REFERENCES subtopics(id),
    question       TEXT NOT NULL,
    correct_answer VARCHAR NOT NULL,
    marks          INT NOT NULL CHECK (marks BETWEEN 1 AND 4),
    trap_type      VARCHAR,
    difficulty     VARCHAR NOT NULL CHECK (difficulty IN ('Easy', 'Medium', 'Hard'))
);

CREATE TABLE choices (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id   UUID NOT NULL REFERENCES questions(id),
    display_order INT NOT NULL,
    label         VARCHAR NOT NULL,
    value         VARCHAR NOT NULL,
    is_correct    BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id),
    session_type VARCHAR NOT NULL CHECK (session_type IN (
                     'todays_action_subtopic', 'todays_action_trap_type',
                     'quiz', 'onboarding'
                 )),
    paper_type   VARCHAR NOT NULL CHECK (paper_type IN ('Paper 1', 'Paper 2')),
    status       VARCHAR NOT NULL CHECK (status IN ('in_progress', 'completed'))
                     DEFAULT 'in_progress',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at     TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE attempts (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID NOT NULL REFERENCES users(id),
    question_id           UUID NOT NULL REFERENCES questions(id),
    session_id            UUID NOT NULL REFERENCES sessions(id),
    choice_id             UUID REFERENCES choices(id),
    is_correct            BOOLEAN NOT NULL,
    correct_answer        VARCHAR NOT NULL,
    student_answer        VARCHAR NOT NULL,
    marks                 INT NOT NULL CHECK (marks BETWEEN 1 AND 4),
    time_started_at       TIMESTAMPTZ NOT NULL,
    time_ended_at         TIMESTAMPTZ NOT NULL,
    time_spent_in_seconds INT NOT NULL CHECK (time_spent_in_seconds >= 0),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CHECK (time_ended_at >= time_started_at)
);

CREATE TABLE user_weaknesses (
    user_id           UUID NOT NULL REFERENCES users(id),
    subtopic_id       UUID NOT NULL REFERENCES subtopics(id),
    error_rate        NUMERIC(5,2),
    trap_error_rate   NUMERIC(5,2),
    last_practiced_at TIMESTAMPTZ,

    PRIMARY KEY (user_id, subtopic_id)
);

CREATE TABLE user_readiness (
    user_id       UUID NOT NULL REFERENCES users(id),
    score         NUMERIC(5,2),
    grade         CHAR(2),
    calculated_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (user_id)
);

CREATE INDEX idx_attempts_user_created ON attempts(user_id, created_at DESC);
CREATE INDEX idx_attempts_question_id ON attempts(question_id);
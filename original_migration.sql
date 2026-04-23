-- A student's answer to one question
CREATE TABLE attempts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL,
    question_id         UUID NOT NULL,
    session_id          UUID NOT NULL,          -- groups attempts from one study session
    subject             VARCHAR NOT NULL,        -- 'Mathematics' or 'Additional Mathematics'
    subtopic            VARCHAR NOT NULL,        -- e.g. 'Quadratic Equations', 'Indices', 'Matrices'
    trap_type           VARCHAR,                 -- e.g. 'Sign Error', 'Concept Confusion', 'Formula Misuse'
                                                 -- NULL if question has no known trap
    is_correct          BOOLEAN NOT NULL,
    student_answer      VARCHAR,                 -- what the student selected or wrote
    marks               INTEGER NOT NULL,        -- marks this question is worth in real SPM (1–4)
    difficulty          VARCHAR NOT NULL,        -- 'Easy', 'Medium', 'Hard'
    paper_type          VARCHAR NOT NULL,        -- 'Paper 1' (MCQ) or 'Paper 2' (Essay)
    time_started_at     TIMESTAMPTZ NOT NULL,
    time_ended_at       TIMESTAMPTZ NOT NULL,
    time_spent_seconds  INTEGER NOT NULL,        -- calculated: time_ended_at - time_started_at
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Aggregated weakness per subtopic per user
-- Recalculated periodically, not in real-time
CREATE TABLE user_weaknesses (
    user_id             UUID NOT NULL,
    subtopic            VARCHAR NOT NULL,
    error_rate          NUMERIC(5,2),            -- % wrong in last 20 attempts for this subtopic
    trap_error_rate     NUMERIC(5,2),            -- % where trap_type was set AND is_correct = false
    last_practiced_at   TIMESTAMPTZ,
    PRIMARY KEY (user_id, subtopic)
);

-- A student's overall readiness score
CREATE TABLE user_readiness (
    user_id             UUID PRIMARY KEY,
    score               NUMERIC(5,2),            -- 0 to 100
    grade               CHAR(2),                 -- 'A+', 'A', 'B', 'C', 'D'
    calculated_at       TIMESTAMPTZ NOT NULL
);

-- A practice session
CREATE TABLE sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL,
    session_type        VARCHAR NOT NULL,        -- 'todays_action_subtopic', 'todays_action_trap_type',
                                                 -- 'quiz', 'onboarding'
    status              VARCHAR NOT NULL,        -- 'completed', 'in_progress'
    paper_type          VARCHAR NOT NULL,        -- 'Paper 1', 'Paper 2'
    started_at          TIMESTAMPTZ NOT NULL,
    ended_at            TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
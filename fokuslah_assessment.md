# Backend Engineer Assessment — Fokuslah

**Estimated time:** 2–3 hours  
**Deadline:** 24 hours from receipt  
**Submission:** Email us a GitHub repo link (private is fine, add [your email]) + your written responses inline in a separate `ANSWERS.md` file

---

## How We Evaluate

We score submissions on 5 dimensions:

| Dimension | What we're looking at |
|---|---|
| **Correctness** | Does it actually work? Does it handle edge cases? |
| **Production safety** | Would you trust this at 10pm with real users on it? |
| **Trade-off reasoning** | Do you know what you're giving up with each decision? |
| **Code clarity** | Can a teammate read this without asking you questions? |
| **User & business awareness** | Do you think about the 16-year-old waiting for an explanation, not just the code? |

There's no perfect score. A candidate who scores 3/5 on correctness but 5/5 on trade-off reasoning is more interesting to us than someone who nails the code but can't explain why.

---

## Deliverable Expectations

- **Go code required** for Part 1 and Part 3 — doesn't need to compile perfectly but should be real Go, not pseudocode
- **Part 2** — you can mock the LLM client, we don't expect working API credentials
- **Database migrations** — optional, but appreciated if you're changing the schema
- **Tests** — optional, but writing tests for the concurrency case in Part 3 or a failure path in Part 2 is a strong signal
- **`ANSWERS.md`** — required, not optional. The written answers matter as much as the code

---

## Before You Start

A few things worth knowing:

Fokuslah is a diagnostic exam prep platform for Malaysian secondary school students. Students practice SPM Math questions, and our system figures out exactly where they're losing marks — which subtopics they don't understand, which question traps they keep falling into, and how their readiness is trending over time.

Our stack: **Golang backend, PostgreSQL (via Supabase), Redis, Next.js frontend, DigitalOcean infra.**

This assessment is based on real problems we've dealt with. There are no trick questions, and there's no single correct answer. We're more interested in how you think than whether your code compiles perfectly.

One request: write like yourself. We'll be doing a follow-up call where we'll ask you to walk through your decisions. If you can't explain it, it won't count.

---

## The Data Model (Read This First)

Here's a simplified version of our core schema. You'll need this for all three parts.

```sql
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
```

**How Readiness Score is calculated:**

The score is based on the student's last 15 attempts across all subtopics:
- Each correct answer = 1 point
- Each incorrect answer where `trap_type IS NOT NULL` = -0.5 points (falling into a known trap is worse than a regular mistake)
- Each incorrect answer where `trap_type IS NULL` = 0 points
- Score = MAX(0, (points / 15) * 100)

Grade thresholds: A+ = 85+, A = 70–84, B = 55–69, C = 40–54, D = below 40

**Note on marks:** Questions are worth 1–4 marks each in the real SPM exam. The readiness score currently does not weight by marks — that's intentional for now, but it's a known limitation you may want to think about.

---

## Part 1 — Build It Right the First Time (60–75 min)

**The situation:**

We're building a `POST /api/sessions/complete` endpoint. When a student finishes a study session, the frontend sends all their answers at once. We need to:

1. Save all the attempts
2. Return an updated weakness summary for the subtopics they just practiced (so the UI can show them what to work on next)

Here's the first draft a junior engineer wrote:

```go
func CompleteSession(w http.ResponseWriter, r *http.Request) {
    var payload struct {
        UserID  string `json:"user_id"`
        Answers []struct {
            QuestionID    string  `json:"question_id"`
            Subtopic      string  `json:"subtopic"`
            TrapType      *string `json:"trap_type"`       // null if no trap
            IsCorrect     bool    `json:"is_correct"`
            StudentAnswer string  `json:"student_answer"`
            Marks         int     `json:"marks"`
            Difficulty    string  `json:"difficulty"`
            PaperType     string  `json:"paper_type"`
            TimeSpentSecs int     `json:"time_spent_seconds"`
        } `json:"answers"`
    } 
    // no validation for payload, gonna break if use invalid data, for example use other paperType that not available in the system. will be brake when validate the answer

    json.NewDecoder(r.Body).Decode(&payload)

    sessionID := uuid.New().String()

    for _, answer := range payload.Answers {
        db.Exec(`
            INSERT INTO attempts (
                user_id, question_id, session_id, subtopic, trap_type,
                is_correct, student_answer, marks, difficulty, paper_type,
                time_spent_seconds, time_started_at, time_ended_at
            )
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
        `, payload.UserID, answer.QuestionID, sessionID, answer.Subtopic,
           answer.TrapType, answer.IsCorrect, answer.StudentAnswer,
           answer.Marks, answer.Difficulty, answer.PaperType, answer.TimeSpentSecs)
    }

    rows, _ := db.Query(`
        SELECT subtopic,
               COUNT(*) as total,
               SUM(CASE WHEN is_correct = false THEN 1 ELSE 0 END) as wrong
        FROM attempts
        WHERE user_id = $1
        GROUP BY subtopic
    `, payload.UserID)

    type WeaknessSummary struct {
        Subtopic  string  `json:"subtopic"`
        ErrorRate float64 `json:"error_rate"`
    }

    var weaknesses []WeaknessSummary
    for rows.Next() {
        var w WeaknessSummary
        var total, wrong int
        rows.Scan(&w.Subtopic, &total, &wrong)
        w.ErrorRate = float64(wrong) / float64(total) * 100
        if w.ErrorRate > 40 {
            weaknesses = append(weaknesses, w)
        }
    }

    json.NewEncoder(w).Encode(weaknesses)
}
```

**Your task:**

1. List every problem you see — correctness bugs, performance issues, anything that will hurt us in production. Be specific. "error handling is missing" is not specific. Tell us *what breaks and when*.

2. Rewrite it. Your version should be production-ready, not just functional. You can introduce new infrastructure (queues, caching, background jobs) if you think it's warranted — but justify each addition. We're a small team; we don't add complexity for fun.

3. The weakness query above scans all attempts for a user. With 50,000 users doing 20 attempts per day, this will become a problem. Describe how you'd fix this — you don't need to implement it fully, but be specific about what you'd change and why.

**One question to answer in `ANSWERS.md`:**

> The junior engineer ran the weakness calculation synchronously — the student waits for it before getting a response. Would you keep it synchronous or move it async? There's no universally correct answer here. Tell us what you'd do and what you'd be giving up.

---

## Part 2 — LLM in Production (45–60 min)

**The situation:**

Our core feature is AI-generated step-by-step explanations. When a student gets a question wrong, we call an LLM to explain why and how to solve it correctly. This is the feature students say they love most — and it's the one that triggers them to upgrade to paid.

Here's what we're currently running:

```go
func GenerateExplanation(ctx context.Context, question, wrongAnswer, correctAnswer, subtopic string) (string, error) {
    resp, err := openaiClient.CreateChatCompletion(ctx,
        openai.ChatCompletionRequest{
            Model: "gpt-4",
            Messages: []openai.ChatCompletionMessage{
                {
                    Role:    openai.ChatMessageRoleUser,
                    Content: fmt.Sprintf("Explain why '%s' is wrong for this question: %s. The correct answer is %s.", wrongAnswer, question, correctAnswer),
                },
            },
            MaxTokens: 1000,
        },
    )
    if err != nil {
        return "", err
    }
    return resp.Choices[0].Message.Content, nil
}
```

**Context that matters:**
- We serve Malaysian secondary school students. Most are 16–17 years old.
- The same SPM questions appear every year. There are roughly 800 unique questions in our bank.
- An explanation for the same question + same wrong answer will be identical every time.
- We're on a tight budget. Token costs are a real concern.
- Students sometimes wait 8–12 seconds for an explanation. They drop off.

**Your task:**

1. Identify at least 4 specific problems with this implementation. For each one, tell us the real-world impact — not just "it's bad practice" but what actually happens to the student or the business.

2. Rewrite it with your fixes. You don't need a working OpenAI client — mock it if needed. We care about the structure, not the credentials.

3. Our explanation quality is inconsistent. Sometimes the LLM gives a university-level answer. Sometimes it's too simple. Rewrite the prompt so it's more likely to produce a consistent, useful explanation for a 16-year-old Malaysian student who just got a Math question wrong.

**One question to answer in `ANSWERS.md`:**

> If our LLM provider goes down at 9pm on a Sunday — peak usage time for students — what happens in your implementation? Walk us through it.

---

## Part 3 — Something Is Wrong in Production (45–60 min)

**The situation:**

It's 10pm. Students are actively using the app. You get a message in Slack:

> *"my readiness score went from 72 to 45 after i got 8/10 correct?? this app is broken"*

Two more messages come in with the same complaint within 5 minutes.

Here's the function that updates a student's readiness score. It runs after every attempt is saved:

```go
func UpdateReadinessScore(ctx context.Context, userID string, db *sql.DB) error {
    rows, err := db.QueryContext(ctx, `
        SELECT is_correct, trap_type
        FROM attempts
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT 15
    `, userID)
    if err != nil {
        return err
    }
    defer rows.Close()

    var total int
    var points float64

    for rows.Next() {
        var isCorrect bool
        var trapType *string
        rows.Scan(&isCorrect, &trapType)
        total++
        if isCorrect {
            points++
        } else if trapType != nil {
            points -= 0.5
        }
    }

    if total == 0 {
        return nil
    }

    score := math.Max(0, (points/15)*100)

    grade := scoreToGrade(score)

    _, err = db.ExecContext(ctx, `
        UPDATE user_readiness
        SET score = $1,
            grade = $2,
            calculated_at = NOW()
        WHERE user_id = $3
    `, score, grade, userID)

    return err
}
```

Students can now study on two devices at the same time — mobile and web. This was shipped two weeks ago.

**Your task:**

1. Find the root cause of the score drop bug. Be specific — not just "race condition" but exactly what sequence of events causes a student with 8/10 correct to see their score drop.

2. Fix it. Your fix needs to be safe to deploy at 10pm without taking the system down. Walk us through your reasoning — why is your fix correct and not just *less broken*?

3. There's a second bug in this function that's unrelated to concurrency. It's subtle. Find it and explain what it means for the student experience. Hint: look carefully at the scoring formula against our data model.

4. This function is called after *every single attempt*. At scale, that's a problem. What would you change about this architecture?

**One question to answer in `ANSWERS.md`:**

> You've found the bug and you have a fix ready. It's 10pm. Do you deploy now or wait until morning? Tell us how you make that call.

---

## Submission

Create a repo with:

```
/
├── part1/          # Your rewritten CompleteSession + any supporting files
├── part2/          # Your rewritten GenerateExplanation + prompt
├── part3/          # Your fixed UpdateReadinessScore
└── ANSWERS.md      # Your written responses to the questions in each part
```

Email the repo link to [email]. If it's private, add us as a collaborator.

We'll follow up within 24 hours to schedule a 30-minute walkthrough call. We'll ask you to explain your decisions. That call matters as much as the code.

---

## What We're Actually Looking For

We're not checking if you know the "right" answer. We're checking:

- **Do you understand why something breaks** — not just that it breaks?
- **Do you think about the user** — the 16-year-old waiting 10 seconds for an explanation — not just the code?
- **Do you make reasonable trade-offs** — and can you articulate them?
- **Would we trust you to make a call at 10pm** — without someone senior above you?

If you're unsure about something, say so. Saying "I'm not sure about X, so I made this assumption" is better than pretending you know.

Good luck.

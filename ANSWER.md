# Part 1

## Assumption
- I don't know whether in 1 attempt you can answer questions from more than 1 subtopic, so I assume that 1 attempt only answers questions from 1 subtopic.
- I don't know whether after completing the session it should display all user weaknesses from all topics or only the topics discussed during the assessment, so I assume it only displays weaknesses from the subtopics discussed in the assessment.

## Problem with the Original Implementation
- The payload accepts subtopics as free text, which risks being filled with subtopics that are not relevant to the question being answered, which can lead to unreliable data.
- The payload does not accept time_start and time_end per answer and instead uses `now()` as the time_start and time_end values ​​for the database, which do not reflect the actual time the student worked on and answered the question.
- The payload accepts is_correct and marks, which can be used to manipulate the values, impacting the accuracy of user weaknesses and user readiness analysis.
- Unhandled JSON decoder errors can cause payload variables to contain empty/zero values ​​from defined structs. For example, the subtopic string field will contain an empty string ("") and can be inserted into the database as a valid value.
- Payload accepts userId which means it can input answers on behalf of another user, the data becomes unreliable
- Question IDs are not validated to be present in the database, potentially storing answers that have no associated question, resulting in unusable data and wasting database resources.
- Insert data into the attempts table using loop which has the potential for invalid data if it fails midway because using a loop means all data is inserted using a different transaction, besides that each loop has the potential to open a new connection to the database which if in large numbers can overload the database.
- Transactions are not used for interconnected operations that must be atomic, potentially leading to invalid data because they can be input half-way.
- The weakness query counts all attempts the user has ever made, even though the data in the user_weaknesses table only stores calculations from the last 20 attempts for each topic.
- The weakness query calculates the error rate for all subtopics in a single attempt, even though there may not be any questions related to that topic in the session.
- There is no explanation why only weaknesses with an error rate >= 40% are returned to the user.

## Answer
Based on the above assumptions, I will continue to calculate synchronously so that the data can be immediately displayed to users and they can see their test results, without complicating the architecture.

Furthermore, the sync solution ensures atomic transactions because all queries run in a single transaction. If using async and the calculation process fails, the data will be invalid because the session and attempt data are saved, but user weaknesses are not calculated.However, sync has trade-offs:
- latency, which can increase as the number of subtopics discussed in a single session increases.

Further optimizations, such as using window functions, are required if you want to continue using the sync method. This has the advantage of lower latency but consumes more database memory per query. Another option is async processing, but as explained above, we will face data reliability issues.

## How I Fix Weaknesses Query
I fixed the weaknesses query by only calculating the 20 most recent data according to the provisions in the user_weaknesses table and only calculating weaknesses for the subtopics discussed in the session being worked on.

## What I'm Giving Up
- Decouple Failure: Sync is closely related to transactions, if there is 1 operation that fails then it will rollback all operations including answer checking, the user will get an error even if the answer is correct and this can cause bad UX
- if sessions grow to cover many subtopics, latency grows with them. At that point I'd either optimize the query (collapse N subtopic queries into one using window functions) or move the calc to a background job

# Part 2

## Assumption
- I don't know if it is necessary to explain why/where the user's answer is wrong, so I conclude that it is not necessary to explain why the user's answer is wrong, assuming this makes the cache strategy easier.

## Problems with the Original Implementation

**1. No caching — identical LLM calls repeated endlessly**
The same question + same wrong answer always produces the same explanation, but the code calls the LLM fresh every single time. With ~800 unique questions in the bank, the same GPT-4 call is made over and over. Real impact: every student pays the full 8–12 second wait even for a question that has been explained hundreds of times before. Token costs compound for zero benefit.

**2. No timeout on the LLM call**
The function receives a `ctx` but never sets a deadline on the LLM request. If OpenAI is slow or stalls, the student is stuck on a loading screen indefinitely — there is no upper bound on how long they wait. Real impact: students drop off mid-session, the most engagement-critical feature becomes the one most likely to make them leave.

**3. No retry and no fallback**
When `err != nil` the function returns an empty string and an error. There is no retry, no fallback message, nothing. Real impact: if OpenAI has a 2-minute hiccup at 9pm Sunday (peak usage), every student who gets a question wrong during those 2 minutes sees a broken feature — the exact feature Fokuslah says drives paid upgrades.

**4. Weak prompt — no system message, no audience context, `subtopic` unused**
A single user message with no system prompt gives the LLM no instructions about tone, reading level, or format. The `subtopic` parameter is accepted but never passed into the prompt. Real impact: output quality is inconsistent — sometimes a university-level derivation, sometimes too vague — and the LLM has no idea it is talking to a 16-year-old SPM student. The subtopic would let the LLM focus the explanation on the right concept.

**5. `resp.Choices[0]` without a length check**
If the LLM returns an empty choices array (which can happen), this line panics and crashes the server mid-request. Real impact: an unrecovered panic in Go takes down the whole goroutine and can cascade if not recovered higher up.

## Caching Decision

I cache by `question_id` only, not by `(question_id, student_answer)`.

The explanation covers *how to reach the correct answer*, not a teardown of a specific wrong answer. For a 16-year-old who just got a question wrong, understanding the correct method is more useful than understanding why their specific guess was wrong — and it lets us cache once per question across the entire ~800-question bank regardless of which of many possible wrong answers a student chose. This turns the cache from "useful sometimes" to "near-100% hit rate after warmup".

We can also consider storing explanations generated using PostgreSQL, as the explanations for each question will always be the same. Using PostgreSQL eliminates cache expiration and eliminates the need to call LLM for expired explanations in Redis. This way, when all questions have explanations, we can reduce the cost of generating explanations by almost 100%. Of course, if more questions are added, LLM will be called again to get the explanations for those questions.
We can also store explanations in Redis without requiring expiration, but because Redis stores data in memory, which is limited and expensive, PostgreSQL can be an alternative.

## What Happens When the LLM Goes Down at 9pm Sunday

**Students who ask about already-seen questions (cache hit):**
They get the cached explanation immediately. The LLM outage is completely transparent for them — no wait, no error, no degraded experience.

**Students who ask about a question not yet in cache (cache miss):**
1. Cache miss → LLM call attempted with a 15-second timeout.
2. LLM is down → call fails after timeout.
3. Service retries up to 2 more times (with 500ms delay between, respecting parent context cancellation).
4. All 3 attempts fail → return `fallbackMessage`: *"We couldn't load an explanation right now. Review the correct answer above and try working through it step by step — you'll get it!"*
5. Student sees a friendly message instead of a blank error screen or a spinner that never resolves.
6. The fallback is **not** cached — when the LLM recovers, the next student to hit that question gets the real explanation, which then populates the cache for everyone after them.

No manual intervention is needed to recover. Once the provider is healthy again, the cache self-heals request by request.

# Part 3

## Root Cause of the Score Drop Bug

The bug is a **lost update race condition** caused by two concurrent goroutines reading the same stale snapshot and then overwriting each other.

Exact sequence of events (student uses mobile + web simultaneously):

1. Student submits answer on **mobile** → attempt row inserted → `UpdateReadinessScore` called.
2. Student submits answer on **web** at roughly the same time → attempt row inserted → `UpdateReadinessScore` called.
3. Both goroutines execute `SELECT is_correct, trap_type FROM attempts ... ORDER BY created_at DESC LIMIT 15` at the same moment, before either `UPDATE user_readiness` has committed. They both read the **same snapshot** — the 15 most recent attempts from before this session.
4. Both goroutines compute a score independently from that same snapshot.
5. Both execute `UPDATE user_readiness SET score = $1 ...`. The second `UPDATE` overwrites the first.

The student got 8/10 correct in this session. But if, say, the web device's goroutine ran its `UPDATE` last and happened to read a snapshot that still reflected an older bad session (because the mobile device's fresh good attempts weren't committed yet when it read), the final stored score is the stale one. The student sees their score drop even though they did well.

## The Fix

Wrap the entire read-then-write in a single transaction and take a **`SELECT ... FOR UPDATE`** lock on the student's `user_readiness` row before reading attempts.

```sql
-- Step 1: ensure the row exists (new students won't have one)
INSERT INTO user_readiness (user_id, score, grade, calculated_at)
VALUES ($1, 0, 'D', NOW())
ON CONFLICT (user_id) DO NOTHING;

-- Step 2: lock the row
SELECT 1 FROM user_readiness WHERE user_id = $1 FOR UPDATE;

-- Step 3: read attempts (now inside the same transaction, after the lock)
SELECT is_correct, trap_type FROM attempts WHERE user_id = $1 ORDER BY created_at DESC LIMIT 15;

-- Step 4: update
UPDATE user_readiness SET score = $1, grade = $2, calculated_at = NOW() WHERE user_id = $3;
```

**Why this is correct, not just less broken:**

`FOR UPDATE` is a row-level lock. The second goroutine (whichever device arrives second) blocks at step 2 until the first goroutine commits. By the time the second goroutine proceeds to step 3, the first goroutine's attempt is already visible in the `attempts` table. Both goroutines now compute their score from a complete, up-to-date snapshot — no stale reads, no overwrites.

There is no schema change and no new infrastructure required. It is safe to deploy at 10pm because:
- It is a pure code change to an existing function.
- There is no data migration.
- The lock only serialises concurrent calls for the **same user**, so it does not affect throughput for other users.
- Rollback is a one-binary revert.

## Second Bug (Unrelated to Concurrency)

The formula always divides by **15** as the denominator, even when the student has completed fewer than 15 total attempts.

```go
score := math.Max(0, (points/15)*100)
```

A new student who aces their first 3 questions gets: `(3 / 15) × 100 = 20` — **Grade D** — even though they answered everything correctly. The `total` variable is tracked but used only for the `if total == 0` early-return guard; it is never used as the denominator.

**Impact on student experience:** Students who are new or returning after a long break will see an artificially deflated score on their first session regardless of how well they performed. For a 16-year-old who just got 3/3 and sees Grade D, this is discouraging and erodes trust in the platform — exactly the opposite of what an onboarding experience should do.

**Fix options:**
- Don't show a readiness score at all until the student has at least N attempts (e.g. 5). Show a "keep practicing to unlock your score" message instead.
- Divide by `min(total, 15)` so the score reflects actual accuracy when history is short, then normalises to the 15-attempt window as data grows.

The test `TestComputeScore_NewStudentScoreDeflation` in `readiness_test.go` documents this behaviour.

## Architecture at Scale

Calling `UpdateReadinessScore` after every single attempt means 50,000 users × 20 attempts/day = 1,000,000 score recalculations/day (~12/sec average, much higher at peak). Each recalculation holds a row lock and does a `SELECT` + `UPDATE`. Under load this creates a lock queue per active user and saturates the database.

**What I would change:**

Move to a **debounced background job** via a queue (Redis `LPUSH` or a lightweight queue table):

1. After an attempt is saved, push `recalculate:{user_id}` onto a deduplicated queue (e.g. Redis set — if the key already exists, the push is a no-op).
2. A background worker polls the queue and processes each `user_id` once, regardless of how many attempts came in during the window. Deduplication collapses N attempts in the same session into 1 recalculation.
3. Readiness score lags behind by a few seconds at most, which is acceptable — the student's score doesn't need to update between individual questions.

This reduces DB write pressure from N_attempts to 1_per_session, eliminates the hotspot under peak load, and the deduplication also naturally prevents the race condition (one worker per user at a time).

## Deploy Decision: Now or Morning?

**I deploy now.**

The bug is actively harming real users at peak usage time. Every minute it runs, more students see a wrong score and lose trust in the product. The fix is surgical — it adds a transaction and a row lock to an existing function, touches no schema, requires no data migration, and can be rolled back in under a minute by reverting the binary.

The decision framework I use:

| Factor | Assessment |
|---|---|
| Is the fix self-contained? | Yes — one function, no migration |
| Is rollback fast? | Yes — revert binary, no DB changes to undo |
| Is the current bug causing active harm? | Yes — 3+ complaints in 5 minutes at 10pm peak |
| Does waiting until morning reduce risk? | Marginally — but the bug continues hurting users all night |

I would: confirm the fix locally, deploy to production, watch error rates and score update latency for 10 minutes, and keep the rollback command ready. If anything looks wrong, revert immediately. If clean, close the incident and write the postmortem in the morning.

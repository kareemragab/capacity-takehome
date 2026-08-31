# The Capacity Problem

> ## ⚠️ READ THIS FIRST: the transcript is part of the deliverable
>
> **You must hand in the transcript of your AI agent sessions.** Committed to
> the repo, in `transcripts/`, linked from your README. See
> [Hand in](#hand-in).
>
> **No transcript is a deal breaker.** Not a smaller score. We close the tab,
> whatever the code looks like. This is the one rule with no judgement call in
> it, and it is the most common way people fail this exercise.

Build a small social app where the contact list has a hard ceiling. The features
are simple. The rules underneath them are not — that's the exercise.

**4 hours.** Use AI agents freely; we do, and you hand in the transcript. It
ends with a 45-minute call where you demo it and make one live change.

Four hours will not comfortably fit everything below, and it isn't meant to.
What you choose to build first, and what you consciously drop, is part of what
we're reading. R1–R6 is the core; R7 and R8 exist for the unlikely case you're
early. Say in your README what you left and why.

## Run it

Needs Go 1.25+, Node 20+, Docker.

```bash
make up        # mongo on :27117 (replica set, so transactions work)
make api       # graphql on :8080, seeds ten users on first boot
make mobile    # expo — press i for the iOS simulator, w for web
make check     # go build + vet + test, and tsc on the client. green on clone
```

Playground at <http://localhost:8080>. Confirm it's alive:

```bash
curl -s localhost:8080/query -H 'Content-Type: application/json' \
  -d '{"query":"{ users { id name } }"}'
```

Copy any `id` and send it as the `X-User-Id` header. That's the whole
authentication story and it's intentionally fake.

## Stack

| | |
|---|---|
| API | Go 1.25 · GraphQL (gqlgen) · MongoDB 8 |
| Client | Expo SDK 57 · React Native · TypeScript |
| Local | Docker |

Never written Go or shipped an Expo app? Not a disqualifier — getting productive
in an unfamiliar stack inside a day is part of what we're measuring. If mobile
genuinely blocks you, a web client is fine; say so.

## Tiers

| Tier | Cap |
|---|---|
| Pink flag | 1 |
| Blue flag | 3 |
| Green flag | 5 |
| **Shared budget** | **8** |

Sub-caps sum to 9. The budget is 8. That gap is deliberate.

## Build

| | | |
|---|---|---|
| **R1** | Send a request to a named tier. | core |
| **R2** | Accept / decline. Accept creates the contact on both sides. | core |
| **R3** | Move a contact between tiers. | core |
| **R4** | Remove a contact. Frees the seat on both sides. | core |
| **R5** | People screen: contacts by tier, live `used / cap`, budget visible. | core |
| **R6** | Request inbox. A failed accept says *why*, in plain words. | core |
| **R7** | Posts scoped to a tier and everything closer. | stretch |
| **R8** | Optimistic accept with rollback. | stretch |

**Out of scope, don't spend time here:** auth, signup, profiles, search, push,
deployment, visual polish.

## The four rules

Most of the grade is here, not in the screens.

**1. Budget before sub-cap.** 3 Blue + 5 Green is 8 of 8. That person cannot add
a Pink flag, even though Pink is empty. The sum is checked first.

**2. A pending request holds no seat.** Sending creates no contact. One free
seat buys unlimited outstanding requests. Capacity is checked at **accept**,
against **both** people — either side being full fails it.

**3. Re-filing is not adding.** Moving Green → Blue checks the destination
sub-cap only, never the budget. The contact is already inside the budget; a
budget check here blocks a legal move.

**4. Two people can want the last seat.** Concurrent accepts on one free seat:
exactly one wins, the other fails cleanly. Read-then-write is not an answer.

Also: `used` may legally exceed `cap` (a lowered cap, a merge). Fail closed,
don't panic. And caps are config, not constants — already done for you in
`api/internal/config`. Don't undo it.

## Where to start

`api/internal/capacity/capacity.go` — three functions returning
`errNotImplemented`, and a test file with five `t.Skip`ed tests named after the
rules they should prove. Delete a Skip, write the test, make it pass. Rule 4
needs a real database, so its stub is in `api/internal/store/race_test.go`.

Then work outwards: resolvers in `api/graph/schema.resolvers.go` (`me` and
`users` work, the rest panic), screens in `mobile/`.

```
api/graph/schema.graphqls      the API surface. edit it, then `make generate`
api/internal/capacity/         THE GRADED SURFACE. pure rules, no IO
api/internal/store/            mongo: documents, connection, indexes, seed
mobile/App.tsx                 a user switcher, so you can act as anyone. replace it
```

Worth knowing: Mongo is on **27117**, not 27017, so it can't collide with one
you already run. Caps are env vars — try `CAP_GREEN=500 make api`, nothing
should need recompiling. On a physical device set
`EXPO_PUBLIC_API_URL=http://<your-lan-ip>:8080/query`.

## Hand in

Two things get handed in: **the code, and the transcript.** A repo with only the
code is half a submission.

Replace this README with your own. Keep run instructions, and add:

- **Decisions** — 3 or 4 calls you made and what you rejected. We read this first.
- **What's next, and what's unfinished.** Unfinished with a reason beats hand-waved.
- **Your agent transcript.** Required. Details below.
- **Where the agent got it wrong** — one thing your AI tooling got confidently
  wrong and how you caught it. Can't name one? We'll assume you didn't check.
- Tests for the rules.

### The agent transcript (required)

Commit the full transcript of every AI session you used on this exercise, and
link it from your README under a heading called **Agent transcript**.

This is a deal breaker, not a preference. A submission without a transcript does
not get read, does not get a call, and does not get feedback, no matter how good
the code is. If you are unsure whether what you have counts, send it and ask.

We are not checking whether you used an agent. We assume you did. We are reading
*how*: what you asked for, what came back wrong, where you stopped and thought,
what you threw away. That is the part of your work the finished code hides.

- Put the files in `transcripts/` at the repo root. Raw text or markdown is fine.
- Every tool has an export. Claude Code: `/export`, or the session files under
  `~/.claude/projects/`. Cursor and Copilot: export the chat. ChatGPT and Claude
  web: share link plus a pasted copy, because links rot.
- Do not tidy it. Dead ends, bad prompts and reversals are the signal. A polished
  transcript reads as a rewritten one.
- Redact secrets and anything not yours. Nothing else.
- Worked without an agent for part or all of it? Say that in the README and skip
  the file for that part. Silence is what fails, not abstinence.

## The call — 45 minutes

We read your transcript before this call, and some of the 20 minutes comes
straight out of it. No transcript in the repo, no call.

- **10 min** — you demo it, including one refusal.
- **20 min** — two features in depth. Why this shape, what you rejected, where it
  breaks first.
- **15 min** — one live change to your own code, on the call, with your normal tools.

Graded on: the four rules holding (proven by a test), the rule living in one
pure testable place, refusals reaching the user as a sentence, cutting the right
scope and saying so, how you drove the agent, and — outweighing the rest —
being able to change and explain your own code live.

Questions before you start are free. Ambiguity you resolved and wrote down beats
a question you didn't ask.

## Before you send it

- [ ] `make check` is green on a fresh clone.
- [ ] Tests prove the four rules.
- [ ] README has Decisions, what's unfinished, and where the agent got it wrong.
- [ ] **`transcripts/` exists, has your real sessions in it, and your README
      links to it.** Check this one last. It is the one we get missing every time.

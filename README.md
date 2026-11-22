# EvalGate

AI product teams need to know whether a new assistant response is merely different or unsafe to release. EvalGate compares a baseline response set with a candidate, runs the same checks against both, and shows reviewers exactly what changed.

The included demo makes that problem concrete. A customer asks in Arabic whether an order returned after 10 days is eligible for a refund. `support-v1` answers with the current 14-day policy and cites `refunds-v2`; `support-v2` changes the window to 7 days and cites the older `refunds-v1` source. EvalGate identifies both changes as regressions and blocks the release.

The demo is self-contained. It replays saved responses and recorded latency samples from `fixtures/`; it does not call a model and does not need an API key.

## Quick Start

The simplest Windows setup uses Docker Desktop. From the project directory, run:

```powershell
docker compose up --build
```

Open <http://127.0.0.1:8080>. Compose builds the React frontend and Go server, starts PostgreSQL, and saves run history in the `runs` Docker volume. Stop the app with `Ctrl+C`; add `-d` to the command if you prefer to run it in the background.

### Run without Docker

Install Go 1.25 or newer and Node.js 24, then run:

```powershell
npm ci --prefix web
npm run build --prefix web
go run ./cmd/evalgate serve
```

The server listens on <http://127.0.0.1:8080>. Without `DATABASE_URL`, it writes run history to `data/runs.json`. This file store is intended for one local server process.

## Two-minute demo

1. Leave `support-v1` selected as the baseline and `support-v2` as the candidate, then choose **Run comparison**.
2. Open **Refund eligibility**. The baseline says `14 يوم` and cites `refunds-v2`; the candidate says 7 days and cites `refunds-v1`. The failed **Policy wording** and **Required citation** checks are regressions on a critical case, so the run is **Blocked**.
3. Change the candidate to `support-v3` and run the comparison again. All automatic checks pass, but the run is **Review required** because the Arabic answer in **Escalation tone** changed.
4. Open **Escalation tone**, compare the two answers, and choose **Approve**. The run becomes **Passed**.
5. Reload the page, open **Run history**, and select that run. The approved decision is still attached to the case.
6. Choose **JSON** to export the complete selected run. To compare other saved data, expand **Compare your own JSON files** and select a suite, baseline snapshot, and candidate snapshot. The files in `fixtures/` are working examples of the expected format.

## What decides the release status

EvalGate checks each candidate response for:

| Check | Requirement |
| --- | --- |
| Response schema | A nonempty `answer` and a `sources` array |
| Policy wording | The case's required phrase appears in the answer |
| Required citation | The required source ID appears in `sources` |
| Latency budget | The saved latency is positive and does not exceed the case limit |

A regression means a candidate check failed after the same check passed on the baseline.

- A failed check on a critical case blocks the run.
- A rejected human review blocks the run.
- A changed answer on a case marked for human review stays **Review required** until someone approves or rejects it.
- A failed noncritical check that is not otherwise blocking produces a **Warning**.
- A run passes only after it has no blocking failures or unresolved reviews.

The automatic checks are intentionally narrow. They can confirm an exact policy phrase, source ID, response shape, or recorded latency, but they cannot judge whether rewritten Arabic sounds empathetic and appropriate. That is why **Escalation tone** goes to a person even though its automatic checks pass.

## How the pieces fit together

- **Go** contains the evaluation rules, runs case checks with a bounded worker pool, exposes the HTTP API, and provides the `compare` command. The CLI and browser use the same evaluation package.
- **React and TypeScript** provide the workbench, side-by-side responses, filters, run history, review controls, and JSON import/export.
- **Three.js** renders the release sculpture in the header. Its color responds to a blocked run, and animation stops when the browser reports `prefers-reduced-motion: reduce`.
- **PostgreSQL** stores the full run snapshot, including inputs, responses, checks, and review decisions. Review updates lock the run in a transaction. Docker Compose configures PostgreSQL automatically; local non-Docker mode can use the JSON file store instead.

The application binds to `127.0.0.1` in the provided local setups. It has no accounts or access controls, so do not expose it to a shared network or load real customer data without adding the security and data-handling controls your environment requires.

## Local checks

With Go and Node.js installed, run the same build and test commands used by the workflow:

```powershell
go test -race ./...
npm ci --prefix web
npm run build --prefix web
go build -o bin/evalgate ./cmd/evalgate
```

You can also exercise the release fixtures directly:

```powershell
./bin/evalgate compare -candidate fixtures/support-v2.json
./bin/evalgate compare -candidate fixtures/support-v3.json
```

The first command returns exit code `1` because the run is blocked. The second returns `0`: its automatic gate does not block, although the JSON status remains `Review required` until the browser review is completed. Invalid input or an execution error returns `2`.

If Go is not installed locally, the race tests can run in the same Go image used to build the app. In PowerShell:

```powershell
docker run --rm --mount "type=bind,source=$($PWD.Path),target=/src" -w /src golang:1.25-bookworm go test -race ./...
```

## GitHub Actions

`.github/workflows/verify.yml` defines checks for pushes and pull requests. It installs the Go version from `go.mod` and Node.js 24, then runs the Go race tests, installs locked frontend dependencies, creates the production frontend build, and builds the Go command. Finally, it compares `support-v1` with `support-v3`, writes the result to the workflow summary, and uploads `evaluation.json` as an artifact.

That workflow evaluates the automatic gate only. Human review decisions made in the local workbench are not sent to GitHub. The workflow is included in the repository, but this README does not assume that it has run on a hosted repository.

## Demo data and limits

The six English and Arabic cases are fictional. The response text and latency values are checked-in snapshots, which keeps the result repeatable and makes the demo work offline after dependencies and container images are available. EvalGate does not make live model calls.

## Maintainer

Tasnim Makableh

tasneemmaq89@gmail.com

https://github.com/engTasneemmaq

# `core` asks questions through an Answerer and replays; it saves no sessions

Every operation that changes something is a typed method on `core` (`Add`, `Adopt`, `Remove`, `Unmanage`, `Uninstall`…) that takes an `Answerer`. Each run starts the operation from the beginning, answers each question it reaches from the Answerer, and stops at the first question that has no answer. It returns those questions without changing anything. The terminal prompter, the GUI (fixed answers sent over `aim --json --answers -`), `--yes` (defaults) and CLI flags are all Answerer adapters. Each question has a stable ID (`adopt.version`) plus a `Basis`, a fingerprint of the facts it showed. An answer whose Basis no longer matches is ignored, and the question is asked again. Confirming a plan is the last question, and its Basis fingerprints the whole Plan, so a stale confirmation can never be applied. Changes happen only under the state lock, after the running status and the state generation have been checked again.

`Check` takes no Answerer, so the compiler guarantees that unattended Checks can never prompt.

## Considered Options

- **One `Run(Request)` entry point with fact hashes inside question IDs**: the smallest interface, but results come back untyped, and CLI flags can't map to IDs that change.
- **Sessions saved on disk, with memoised replay**: the most flexible, but adds session storage, cleanup and a determinism burden that AiM doesn't need.

## Consequences

- Question IDs and `Basis` are part of the public `--json` contract (ADR 0001).
- Inside `core`, a Plan is a list of Steps run by one executor, so `--dry-run` and Removal Plans show exactly what a real run would do.
- Replays redo Release lookups, so `source` uses a short-lived lookup cache and ETag requests to stay inside GitHub's unauthenticated rate limit. Downloads are cached by Digest.
- A shared test must run every scenario through both the terminal prompter and the fixed-answers adapter, to catch differences in question order.

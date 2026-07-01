# Phase 3: Retrieval and Answer Quality

## Outcome

Users can trust that answers are grounded in the right sources, cite useful passages, avoid unsupported claims, and fail clearly when the knowledge base does not contain enough evidence.

## Why This Matters

The product is not useful just because a model answers. It is useful when retrieval finds the right evidence, the answer explains the evidence, and the UI makes uncertainty visible.

## Scope

- Retrieval evaluation fixtures.
- Answer evaluation fixtures.
- Citation and source UX.
- Hybrid retrieval options.
- Query rewriting and scoped retrieval.
- Model/provider latency tracking.
- Better default prompt and refusal behavior.

## Implementation Slices

| Slice | Build | Gate |
| --- | --- | --- |
| 3.1 | Evaluation fixture format for documents, questions, expected sources, and expected answer traits. | Done: offline retrieval eval runner reports pass/fail locally. |
| 3.2 | Retrieval metrics: hit recall, source rank, empty result behavior, and latency. | Done: fixture set establishes baseline metrics. |
| 3.3 | Answer metrics: citation presence, unsupported-claim checks, refusal behavior, and latency. | Done: Ask path can be evaluated without manual screenshots. |
| 3.4 | Source preview UX for answers and search hits. | Done: user can inspect cited passages quickly. |
| 3.5 | Hybrid retrieval strategy option. | Lexical plus vector retrieval beats vector-only on fixture set. |
| 3.6 | Model target performance panel. | Admin can compare model latency, errors, and timeout behavior. |

## Dependencies

- Stable ingestion and source metadata from Phase 2.
- Current model gateway abstraction.
- Audit events for ask/search actions.

## Release Gate

- Evaluation runner in CI/local checks.
- Baseline fixture suite documented.
- Browser check for answer source inspection.
- Performance numbers captured for CPU-lite and GPU-local profiles.

## Current Status

- Slices 3.1 through 3.4 now have checked-in retrieval and answer fixtures, Go eval runners, `.\nexus.ps1 eval-retrieval`, `.\nexus.ps1 eval-answer`, release-check coverage, and a source-preview UI for Ask/search passages.
- Retrieval metrics cover aggregate recall, expected rank, latency, and explicit no-hit filter behavior.
- Answer metrics cover expected source grounding, citation text, required/forbidden phrases, refusal wording, prompt context, conversation history, and latency.
- Ask and search results now share selectable source rows with a compact preview panel for the full passage, document/chunk identifiers, score, storage key, and useful metadata.
- The baselines focus on enterprise support and governance behavior: OIDC troubleshooting, HAR diagnostics, PII redaction, MFA-gated status workflows, and scoped empty-result behavior.

## Next Build Candidate

Move to Slice 3.5. Add a hybrid retrieval strategy option and compare lexical plus vector retrieval against vector-only on the fixture set.

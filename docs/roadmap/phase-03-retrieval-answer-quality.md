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
| 3.1 | Evaluation fixture format for documents, questions, expected sources, and expected answer traits. | Eval runner reports pass/fail locally. |
| 3.2 | Retrieval metrics: hit recall, source rank, empty result behavior, and latency. | Fixture set establishes baseline. |
| 3.3 | Answer metrics: citation presence, unsupported-claim checks, refusal behavior, and latency. | Ask path can be evaluated without manual screenshots. |
| 3.4 | Source preview UX for answers and search hits. | User can inspect cited passages quickly. |
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

## First Build Candidate

Start with Slice 3.1. A small eval suite will guide prompt, retrieval, and model choices better than intuition.


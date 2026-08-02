---
name: review-artifact
description: Independently evaluate a structured requirements, architecture, plan, implementation, or verification artifact. Use when a separate reviewer must return an approved, changes_requested, or blocked verdict with evidence-based required changes.
---

# Review artifact

1. Confirm the original request and review subject are present.
2. Check clarity, completeness, feasibility, testability, scope control, and risk handling.
3. Trace each major user outcome to at least one requirement and acceptance criterion.
4. Identify contradictions, unverifiable conditions, missing failure behavior, and unsafe assumptions.
5. Classify findings as critical, high, medium, or low.
6. Put only concrete approval-blocking defects in `required_changes`.
7. Put non-blocking improvements in `optional_suggestions`.
8. Use `blocked` only when review cannot be completed without external input.
9. Keep `open_questions` empty for `approved` and `changes_requested`; include at least one concrete question for `blocked`.
10. Return only data matching the supplied JSON Schema.

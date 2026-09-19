# Repository agent instructions

## Full improvement loop

When the user requests a **full improvement loop**, carry the change through publication rather than stopping at an uncommitted working tree:

1. Read the relevant repository specifications and external standards.
2. Establish the design and acceptance criteria; avoid speculative structure or dependencies.
3. Add or update behavior-focused tests first when behavior changes, and confirm the intended red phase.
4. Implement the smallest coherent change.
5. Run formatting, static analysis, the full test suite, race tests where practical, and representative binary/build checks.
6. Obtain an independent review, fix release-blocking findings, and repeat verification.
7. Update user-facing documentation and release examples.
8. Commit the complete reviewed change and push it to `main`.
9. Wait for required CI on `main` to succeed before tagging.
10. Create the appropriate annotated semantic-version tag and push it.
11. Wait for the release workflow, then download and verify checksums, assets, and embedded version metadata.
12. Close the associated issue only after release verification, and report commit, CI, release, and residual risks.

Do not claim completion while required checks are failing, the release is missing, or the working tree contains unaccounted changes. Do not create a `pkg` directory or additional packages merely to imitate a conventional layout; package boundaries must represent cohesive concepts and real consumers.

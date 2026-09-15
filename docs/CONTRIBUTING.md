# Contributing to Semlia

Thank you for helping build a trustworthy open semantic control plane. Semlia is pre-alpha, so contributions should begin with the confirmed product contract and the task ownership rules rather than an isolated implementation idea.

## Start Here

1. Read the [project SSOT](SSOT.md) and [documentation index](README.md).
2. Check the relevant confirmed plan, work graph, and task packet.
3. Discuss scope before implementing a new capability or changing a public contract.
4. Do not mix unrelated refactors into a governed task.

## Development Setup

Required tools are pinned in `.tool-versions`.

```bash
make doctor
make bootstrap
make test-repository
```

The root Make targets are the stable local interface. CI must call the same underlying commands contributors can run locally.

## Changes

- Add tests before behavior changes and record the expected failing result.
- Keep public API, event, file, CLI, and MCP contracts versioned.
- Include migration, rollback, security, accessibility, and observability evidence when the affected requirement calls for it.
- Update canonical documentation in present tense. Do not create competing definitions of product behavior.
- Never commit credentials, private customer data, personal paths, or generated local state.

## Publication Privacy

Keep machine-wide container inventories, resolved deployment configuration, private account status and raw browser traces outside the repository. Evidence should contain only the project's relevant results and synthetic inputs, with local paths and private identifiers removed. Review commit email addresses and GitHub comment edit history as well as file contents before publication.

Deployment examples must require operator-supplied configuration rather than describe a real installation. Historical evidence may be redacted for privacy and is not an immutable release attestation; use the signed artifact verification process in [Release Verification](operations/release-verification.md).

## Commit Sign-off

Semlia uses the [Developer Certificate of Origin 1.1](https://developercertificate.org/). Sign each commit with:

```text
Signed-off-by: Your Name <your-email@example.com>
```

Git can add the sign-off automatically:

```bash
git commit -s
```

By signing off, you certify that you have the right to submit the contribution under the project's license.

## Review Expectations

Each change is reviewed for:

1. Compliance with the confirmed SSOT, plan, task, and allowed file scope.
2. Correctness, security, performance, maintainability, and test quality.
3. User-facing acceptance, including failure and recovery paths.

Implementation is not accepted until required validation evidence exists and a maintainer explicitly approves it.

## Community Conduct and Security

Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities through the process in the [Security Policy](SECURITY.md), never through a public issue.

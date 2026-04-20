# AI Usage

This project was built with heavy AI assistance (primarily Claude Opus 4.6). This document states what that means.

## What AI did

- **Implementation**: A large portion of the code was written by AI — not as autocomplete, but as a full implementation partner producing entire features, tests, and documentation
- **Speed**: The project went from experiment to working framework in days, which would not have happened without AI
- **Documentation**: Most docs, including this one, were drafted or edited with AI assistance

## What a human did

- **Architecture and direction**: The idea, design constraints, and technical decisions are human-driven
- **Steering**: AI was redirected, corrected, and had wrong turns rejected throughout development
- **Review and cleanup**: The codebase has gone through multiple rounds of manual review — unused code removed, dead abstractions cleaned up, documentation kept in sync with reality
- **Testing**: Test coverage has been examined and improved. Core packages (`template`, `vdom`, `render`, `island`) have 95–100% coverage. Tests for removed features have been cleaned up rather than left as dead weight
- **Quality control**: Code paths are verified for correctness, not just for passing tests

## Current state

The project started as "mostly AI-generated, lightly reviewed" and has evolved into something more collaborative:

- AI remains a heavy contributor to implementation
- Human oversight has grown from steering into active quality control
- Not every line has received a deep manual audit, but the gap is narrowing
- Dead code is actively removed, docs match the code, and architectural decisions are deliberate

## Security context

godom is intended for local UI, not as a public multi-user web framework. The risk profile is different from internet-facing services.

- For local UI experimentation: low risk
- For anything with system access (e.g., `examples/terminal/`): review carefully — it gives full shell access
- For production or serious deployment: review the code yourself

## If you use this project

- The architecture is intentional
- Much of the implementation is AI-generated
- The codebase is maturing but not fully audited
- Use it with understanding, not blind trust

## Credit

A large amount of code was written by AI. Steering and quality control were done by a human. Both contributions were substantial.

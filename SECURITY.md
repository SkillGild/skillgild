# Security policy

SkillGild takes the security of its users, creators and their skills seriously.

## Reporting a vulnerability

**Please don't report security vulnerabilities through public GitHub issues, discussions or pull requests.**

Report them privately in one of these ways:

- Use GitHub's [private vulnerability reporting](https://github.com/SkillGild/skillgild/security/advisories/new) for this repository.
- Email [support@skillgild.dev](mailto:support@skillgild.dev) with "Security" in the subject line.

Please include:

- the affected area: website, API, `skillgild` CLI, MCP server, client libraries or a hosted skill
- steps to reproduce, and the impact you observed
- any proof-of-concept, with secrets and other people's data removed

We'll acknowledge your report, keep you updated while we investigate, and credit you if you'd like once the issue is fixed. Please give us reasonable time to fix the issue before you disclose it publicly.

## Scope

In scope:

- skillgild.dev and api.skillgild.dev
- the `skillgild` CLI and its local MCP server
- the SkillGild TypeScript, Python and Go client libraries
- access controls that protect hosted skill instructions, prompts and server tools

Out of scope:

- denial-of-service or volumetric testing
- social engineering of SkillGild staff, creators or users
- issues in third-party agents such as Claude Code, Codex, Cursor or Gemini CLI. Please report those to their vendors.

## Keeping your credentials safe

`skillgild login` stores a revocable credential in your operating system's credential store. Never paste API keys into chats, MCP config files or repositories. If you think a key has leaked, revoke it right away under **Account → Agent connections** on [skillgild.dev](https://skillgild.dev).

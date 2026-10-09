<div align="center">

<a href="https://skillgild.dev">
  <img src="https://skillgild.dev/brand/skillgild-logo.png" width="88" alt="SkillGild logo" />
</a>

# SkillGild: AI agent skills marketplace

### Your agent is underqualified. Upgrade its skills.

Production-ready **AI agent skills**, tools and developer workflows for **Claude Code, Codex, Cursor, Gemini CLI** and any **MCP** client.<br />
Your agent calls them, SkillGild runs them.

[**Website**](https://skillgild.dev) ·
[**Browse skills**](https://skillgild.dev/skills) ·
[**Docs**](https://skillgild.dev/docs) ·
[**Agent setup**](https://skillgild.dev/docs/quickstart-agents) ·
[**Pricing**](https://skillgild.dev/pricing) ·
[**Learn**](https://skillgild.dev/learn)

[![M8ven Score](https://m8ven.ai/badge/mcp/skillgild-skillgild-k644ng)](https://m8ven.ai/mcp/skillgild-skillgild-k644ng?s=readme)

<a href="https://skillgild.dev">
  <img src="https://skillgild.dev/social-card.png" width="840" alt="SkillGild: Your agent is underqualified. Expert-built AI agent skills. Your agent calls them, SkillGild runs them." />
</a>

</div>

---

## What is SkillGild?

[SkillGild](https://skillgild.dev) is a marketplace for discovering, publishing and monetizing reusable AI agent skills, developer tools and workflows.

- **Developers** install hosted skills into the coding agent they already use and call them through MCP.
- **Creators** [apply to publish](https://skillgild.dev/docs/creators) their own skills and workflows for those agents to use.
- **Teams** invite members to an organization and share the skills the organization unlocks.

Every developer ends up rebuilding the same prompts, workflows and integrations. On SkillGild, someone who knows the job well builds a skill once, and every agent that needs it can reuse it.

> [!NOTE]
> This repository is SkillGild's public home on GitHub: product overview, community guidelines, and the place to report issues and request features. It does **not** contain the SkillGild application source code or the contents of any marketplace skill.

## Watch: SkillGild in 60 seconds

<a href="https://skillgild.dev/about">
  <img src="./assets/skillgild-launch-film.gif" width="840" alt="Opening of the SkillGild launch film: 'Your agent is underqualified', then 'Meet SkillGild', discovering skills such as Brag, LiveCanvas, Logo Design and Academic Plotting, and installing one with the skillgild CLI" />
</a>

<sub>The opening of the launch film. <a href="https://skillgild.dev/about">Watch the full film on the About SkillGild page</a>, or <a href="https://media.skillgild.dev/media/site/brand/skillgild-launch-film-720p.mp4">download the MP4</a>.</sub>

## Supported agents

One SkillGild account and one connection work across agents.

<table>
  <tr>
    <td align="center" width="140">
      <a href="https://skillgild.dev/learn/install-claude-code-skills"><img src="https://skillgild.dev/images/agents/claudecode-color.svg" width="40" height="40" alt="Claude Code logo" /></a><br />
      <a href="https://skillgild.dev/learn/install-claude-code-skills"><b>Claude Code</b></a>
    </td>
    <td align="center" width="140">
      <a href="https://skillgild.dev/learn/install-codex-skills"><img src="https://skillgild.dev/images/agents/codex-color.svg" width="40" height="40" alt="OpenAI Codex logo" /></a><br />
      <a href="https://skillgild.dev/learn/install-codex-skills"><b>Codex</b></a>
    </td>
    <td align="center" width="140">
      <a href="https://skillgild.dev/learn/cursor-mcp-setup">
        <picture>
          <source media="(prefers-color-scheme: dark)" srcset="./assets/agents/cursor-dark.svg" />
          <img src="https://skillgild.dev/images/agents/cursor.svg" width="40" height="40" alt="Cursor logo" />
        </picture>
      </a><br />
      <a href="https://skillgild.dev/learn/cursor-mcp-setup"><b>Cursor</b></a>
    </td>
    <td align="center" width="140">
      <a href="https://skillgild.dev/learn/gemini-cli-skills"><img src="https://skillgild.dev/images/agents/geminicli-color.svg" width="40" height="40" alt="Gemini CLI logo" /></a><br />
      <a href="https://skillgild.dev/learn/gemini-cli-skills"><b>Gemini CLI</b></a>
    </td>
    <td align="center" width="140">
      <a href="https://skillgild.dev/learn/cli-agent-skills">
        <picture>
          <source media="(prefers-color-scheme: dark)" srcset="./assets/agents/mcp-dark.svg" />
          <img src="https://skillgild.dev/images/agents/mcp.svg" width="40" height="40" alt="Model Context Protocol (MCP) logo" />
        </picture>
      </a><br />
      <a href="https://skillgild.dev/learn/cli-agent-skills"><b>Any MCP client</b></a>
    </td>
  </tr>
</table>

Claude Code, Codex, Cursor and Gemini CLI connect through SkillGild's local MCP server. Any other client that can run a local stdio MCP server works too.

## What you can do

| | |
|---|---|
| **Discover AI agent skills** | [Explore AI agent skills](https://skillgild.dev/skills) by category, price and free runs, or start from a hand-picked [collection](https://skillgild.dev/collections). Categories include [Development](https://skillgild.dev/categories/development), [Design](https://skillgild.dev/categories/design), [Research](https://skillgild.dev/categories/research), [Video & Media](https://skillgild.dev/categories/video-media) and [Content Creation](https://skillgild.dev/categories/content-creation). |
| **Run free and Pro skills** | Free skills include a monthly run allowance per skill. [SkillGild Pro](https://skillgild.dev/pricing) adds premium skills and server tools. Your AI subscription stays separate. |
| **Use the SkillGild CLI** | One `skillgild` command handles browser sign-in, installs skill wrappers and runs the local MCP server your agent talks to. |
| **Connect through MCP** | Your agent can search for skills, start a session, call server tools and end the session through the [MCP tools](https://skillgild.dev/docs/quickstart-agents). |
| **Build on the API** | Call skills from your own code with the [REST API](https://skillgild.dev/docs/api) and the TypeScript, Python and Go client libraries. See the [developer quickstart](https://skillgild.dev/docs/quickstart-developers). |
| **Publish a skill** | [Become a creator](https://skillgild.dev/docs/creators): apply, create a skill, write its protected prompt and submit it for review. |
| **Share with your team** | Organizations let owners invite members and share the skills the organization unlocks. |
| **Refer and earn** | The [affiliate program](https://skillgild.dev/referral) pays commission on the Pro payments of people you refer. |

## Install and connect

You set this up once per machine. After that, every skill is available to every connected agent. Full instructions: [Connect your agent](https://skillgild.dev/docs/marketplace#connect-your-agent).

### 1. Install the SkillGild CLI

The `skillgild` CLI is both the sign-in tool and the local MCP server your agent talks to. It is a single binary with no dependencies.

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/SkillGild/skillgild/main/scripts/install-cli.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/SkillGild/skillgild/main/scripts/install-cli.ps1 | iex
```

The installers download the latest [release](https://github.com/SkillGild/skillgild/releases) for your platform and check it against `checksums.txt` before installing. You can also download an archive from the releases page and put `skillgild` on your PATH.

### 2. Sign in

```bash
skillgild login
```

The CLI opens a device page in your browser. Check that the code matches (for example `ABCD-EFGH`) and click **Approve**. A revocable credential is saved to your operating system's credential store, never to a config file. Confirm with:

```bash
skillgild status
```

### 3. Install a skill

```bash
skillgild install brag --agent claude-code
```

Use `--agent codex`, `cursor`, `gemini-cli`, or `shared` (the default) to match your client. This example installs [Brag](https://skillgild.dev/skills/brag), a free skill that turns your project into a short launch video.

### 4. Register the MCP server with your agent

<details open>
<summary><b>Claude Code</b></summary>

```bash
claude mcp add --scope user skillgild -- $(command -v skillgild) mcp
```

Guide: [Install Claude Code skills](https://skillgild.dev/learn/install-claude-code-skills)
</details>

<details>
<summary><b>Codex CLI</b></summary>

```bash
codex mcp add skillgild -- $(command -v skillgild) mcp
```

Guide: [Install Codex skills](https://skillgild.dev/learn/install-codex-skills)
</details>

<details>
<summary><b>Gemini CLI</b></summary>

```bash
gemini mcp add --scope user skillgild $(command -v skillgild) mcp
```

Guide: [Gemini CLI skills](https://skillgild.dev/learn/gemini-cli-skills)
</details>

<details>
<summary><b>Cursor</b></summary>

Add this to `~/.cursor/mcp.json`, or to `.cursor/mcp.json` in a project. Replace the path with the output of `command -v skillgild`:

```json
{
  "mcpServers": {
    "skillgild": {
      "type": "stdio",
      "command": "/absolute/path/to/skillgild",
      "args": ["mcp"]
    }
  }
}
```

Guide: [Cursor MCP setup](https://skillgild.dev/learn/cursor-mcp-setup)
</details>

<details>
<summary><b>Any other MCP client</b></summary>

Start `/absolute/path/to/skillgild mcp` as a local stdio MCP server.

Guide: [Add skills to a coding agent](https://skillgild.dev/learn/cli-agent-skills)
</details>

> [!TIP]
> Apps launched from an IDE may not inherit your terminal's `PATH`, so use the absolute path from `command -v skillgild`. Never put an API key in MCP JSON or TOML, or in a repository. The MCP process reads its credential from the OS credential store.

Restart or reload your agent session. You should then see the `skillgild_search_skills`, `skillgild_start_session`, `skillgild_call_tool` and `skillgild_end_session` tools.

**Prefer to let your agent do the setup?** Paste this into Claude Code, Codex, Cursor or Gemini CLI:

```text
Please set up SkillGild for me by following https://skillgild.dev/docs/quickstart-agents
Explain each step in plain language and ask me before running any command.
```

## The MCP server

`skillgild mcp` is a local stdio MCP server. It gives your agent five tools:

| Tool | What it does |
| --- | --- |
| `skillgild_search_skills` | Search the catalog by task |
| `skillgild_run_skill` | Run a prompt skill and return its result |
| `skillgild_start_session` | Start a session for a hybrid skill and receive its instructions |
| `skillgild_call_tool` | Call one of that session's server tools |
| `skillgild_end_session` | End the session |

**Claude Desktop:** download [`skillgild.mcpb`](https://github.com/SkillGild/skillgild/releases/latest/download/skillgild.mcpb) and open it; Claude Desktop installs the server and asks for your API key. It is also listed in the [MCP Registry](https://registry.modelcontextprotocol.io) as `io.github.SkillGild/skillgild`.

The source is in [`cli/`](cli/) (Go, MIT). Build it with `cd cli && go build ./cmd/skillgild`, or run it in a container with an API key from your [account](https://skillgild.dev/account):

```sh
docker build -t skillgild .
docker run -i --rm -e SKILLGILD_API_KEY skillgild
```

## How hosted skills work

<p align="center">
  <img src="https://media.skillgild.dev/media/site/learn/skills-vs-mcp-flow-880x360.svg" width="760" alt="How a SkillGild skill reaches your coding agent: Claude Code, Codex, Cursor or Gemini CLI talks to the local skillgild MCP server, which calls the hosted runtime over HTTPS, where the private skill runs. MCP is the connection; the skill is the method." />
</p>

<p align="center">
  <img src="https://media.skillgild.dev/media/site/learn/run-lifecycle-880x340.svg" width="760" alt="Lifecycle of one hosted SkillGild run in four steps: authenticate and check access, validate input and reserve a run, run the skill, return the result. If any step fails, the reserved run is released." />
  <br />
  <sub>What happens on SkillGild's servers during one run. If any step fails, the reserved run is not counted.</sub>
</p>

1. **The wrapper.** `skillgild install` writes a small `SKILL.md` wrapper into your agent's skills folder. It holds public metadata and tells your agent when to use the skill and how to start it. It contains no private implementation.
2. **Access check.** When a task matches, your agent starts a session over MCP. SkillGild checks your account, entitlement and allowance first. Only then does it deliver the skill's protected instructions for that session.
3. **Hybrid execution.** Your agent does the work with its own model and tools, in your project. It calls SkillGild server tools for the private steps and shows you a diff before applying changes.

Protected prompts and server tool code are encrypted at rest and are not included in the installed wrapper. Hosted skills need an internet connection. To understand how this differs from an MCP server or a local skill folder, read [Agent skills vs MCP servers](https://skillgild.dev/learn/agent-skills-vs-mcp) and [Free GitHub agent skills vs SkillGild](https://skillgild.dev/learn/skillgild-vs-free-github-skills).

## For creators

Built a workflow that other developers' agents could use? [Publish a skill on SkillGild](https://skillgild.dev/docs/creators):

1. Sign in with Google or GitHub and apply in **Account → Creator studio**.
2. Create a skill with a public listing and input schema.
3. Write its protected prompt. It is encrypted when you save it and is never shown to callers.
4. Submit it for review, then ship updates as new versions.

The SkillGild team reviews every creator application and skill. Upcoming creator features, including the creator dashboard, analytics and payouts, are tracked on the [roadmap](https://skillgild.dev/docs/roadmap).

## Pricing

- **Free skills:** sign in with Google or GitHub and run free skills with a monthly allowance that resets every month.
- **SkillGild Pro:** premium skills and server tools, available monthly, quarterly or yearly. Prices and checkout follow your billing country.
- **Organizations:** invite your team and share the skills your organization unlocks.

See [pricing](https://skillgild.dev/pricing) for current plans.

## Guides and resources

**Get started**
- [Quickstart for everyone](https://skillgild.dev/docs/quickstart): no coding needed
- [Quickstart for developers](https://skillgild.dev/docs/quickstart-developers): API keys, sessions and SDKs
- [Quickstart for AI agents](https://skillgild.dev/docs/quickstart-agents): CLI and MCP setup written for agents
- [Core concepts](https://skillgild.dev/docs/concepts): free runs, paid access and devices
- [API and execution reference](https://skillgild.dev/docs/api)

**Learn about agent skills**
- [Claude skills: examples, setup and workflows](https://skillgild.dev/learn/claude-skills)
- [Best free Claude Code skills](https://skillgild.dev/learn/best-claude-code-skills)
- [Claude Code skills for developers](https://skillgild.dev/learn/claude-code-skills-for-developers)
- [Agent skills vs MCP servers](https://skillgild.dev/learn/agent-skills-vs-mcp)

**Workflow guides**
- [Spec-driven development with OpenSpec](https://skillgild.dev/learn/openspec)
- [Claude Code frontend design skills](https://skillgild.dev/learn/claude-code-frontend-design-skills)
- [Publication-quality research plots with Claude Code](https://skillgild.dev/learn/claude-code-research-plots)
- [Claude Code video skills](https://skillgild.dev/learn/claude-code-video-skills)

**Compare**
- [Best Claude and AI agent skill marketplaces](https://skillgild.dev/learn/best-ai-agent-skill-marketplaces)
- [SkillGild vs free GitHub agent skills](https://skillgild.dev/learn/skillgild-vs-free-github-skills)
- [SkillGild vs Agensi](https://skillgild.dev/learn/skillgild-vs-agensi)
- [SkillGild vs Skillry](https://skillgild.dev/learn/skillgild-vs-skillry)

[Browse all guides →](https://skillgild.dev/learn)

**For agents and tools**
- [`llms.txt`](https://skillgild.dev/llms.txt): site summary for LLMs and agents

## Community and support

- **Bugs and feature requests:** [open an issue](https://github.com/SkillGild/skillgild/issues/new/choose) in this repository.
- **Account, billing or a misbehaving skill:** use the [support center](https://skillgild.dev/support) or email [support@skillgild.dev](mailto:support@skillgild.dev).
- **Security issues:** please don't open a public issue. See [SECURITY.md](./SECURITY.md).
- **Follow along:** [X](https://x.com/skillgilddev) · [LinkedIn](https://www.linkedin.com/company/skillgilddev/) · [Product Hunt](https://www.producthunt.com/products/skillgild)

Before contributing, please read [CONTRIBUTING.md](./CONTRIBUTING.md) and our [Code of Conduct](./CODE_OF_CONDUCT.md).

## License

The contents of this repository are available under the [MIT License](./LICENSE). The license does not cover the SkillGild service, the SkillGild name and logo, or any skill published on the SkillGild marketplace. Each skill carries its own license, shown on its listing.

<div align="center">
<br />
<a href="https://skillgild.dev"><b>skillgild.dev</b></a>: the marketplace for reusable AI agent skills, tools and developer workflows.
</div>

# muster

A CLI that consolidates AI-assisted development workflows into a single tool. It manages the full lifecycle of roadmap items — from planning through implementation, review, and release — using AI coding agents orchestrated through configurable pipelines.

muster replaces a fragmented system of shell scripts, Claude Code skills, Docker configs, and lifecycle hooks with one coherent interface: one tool, one install, one workflow.

## Why "muster"?

The name maps naturally to every phase of the workflow:

- **"Muster" as a verb** — gather, assemble, dispatch. The workflow gathers roadmap items, assembles a plan, and dispatches agents to execute it.
- **"Pass muster"** — the verify and review gates are literally about whether work passes muster.
- **Military muster** — roll call before a mission. Fits the batched, sortie-like execution model where items are selected, checked, and sent out.

**"Muster in"** and **"muster out"** are real terms: *muster in* means to enlist into active service, *muster out* means to discharge upon completion. Items enter the pipeline with `muster in` and leave it with `muster out`.

## Commands

```
muster in [slug]          Full development pipeline for a roadmap item
muster in --next          Advance one step at a time
muster out [slug]         Post-PR lifecycle: monitor CI, ensure merge, cleanup
muster plan [slug]        Standalone research and planning
muster code               Launch an interactive coding agent with workflow skills
muster code --yolo        ...in a sandboxed container
muster status             Show all roadmap items and their state
muster add <description>  AI-assisted item creation
muster sync               Reconcile README and roadmap data
muster down [slug]        Tear down containers
muster init               Set up a project for muster
muster doctor             Health check project configuration
muster update             Self-update from GitHub Releases
```

## Pipeline

`muster in` walks a roadmap item through a configurable pipeline:

**worktree** → **setup** → **plan** → **execute** → **verify** → **review** → **prepare** → **finish**

Each step can use a different AI tool, provider, and model — configured per-project with per-user overrides. Run the full pipeline at once, advance one step at a time with `--next`, or process all eligible items in batch with `--all`.

## Configuration

muster uses a two-layer config system:

- **User config** (`~/.config/muster/config.yml`) — defines available tools, providers, model tiers, and defaults.
- **Project config** (`.muster/config.yml`) — pipeline step overrides, merge strategy, lifecycle hooks. Container settings live in `.muster/dev-agent/config.yml`. Both support `.local.yml` overrides that are gitignored.

## Status

Early development. See [docs/design.md](docs/design.md) for the full design document.

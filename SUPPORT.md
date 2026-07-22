# Getting help

This project is maintained by a small team. Using the right channel helps us respond faster.

## I have a question about how to use sightmaps

→ **[GitHub Discussions](https://github.com/sightmap/sightmap/discussions)**

Examples:

- "How do I model a multi-tenant route?"
- "Can I split one view across multiple files?"
- "What does `memory:` do if the agent doesn't support it?"

Please search existing discussions before posting. The [documentation](https://docs.sightmap.org) answers most how-to questions.

## I think I found a bug

→ **[Open an issue](https://github.com/sightmap/sightmap/issues/new/choose)** using the **Bug report** template.

Before filing:

- Check [open issues](https://github.com/sightmap/sightmap/issues) for duplicates
- Try to reduce the problem to a minimal sightmap that demonstrates it
- Note which version of the spec (`version:` field) you're using, and — for CLI bugs — the `sightmap` version

## I want to propose a change to the spec

→ Start a **Discussion** first. If it gains traction, follow the [SEP process](spec/seps/README.md).

Small doc/wording changes can skip this — just open a PR.

## I want to build a tool or SDK that reads sightmaps

Great. You don't need permission. A few pointers:

- The authoritative schema is [`spec/v1/sightmap.schema.json`](spec/v1/sightmap.schema.json)
- The reference implementation is under [`go/`](go/)
- Reference examples live in [`spec/v1/examples/`](spec/v1/examples/)
- Subscribe to issues labeled [`spec-change`](https://github.com/sightmap/sightmap/labels/spec-change) to stay aware of upcoming changes
- When your tool is ready, open a PR to list it on sightmap.org

## I'm a Subtext customer and I need help with Subtext

→ This repo is not the right place. Contact Subtext support through your normal channel or visit [subtext.fullstory.com](https://subtext.fullstory.com).

## Something urgent

- **Security issues** — see [`SECURITY.md`](SECURITY.md)
- **Code of Conduct concerns** — see [`CODE_OF_CONDUCT.md`](https://github.com/sightmap/.github/blob/main/CODE_OF_CONDUCT.md)

## Response times

Best-effort. The maintainer team aims for:

- First response on new issues: **3 business days**
- First response on PRs: **3 business days**
- Discussions: no SLA — we read them all but reply when we can

We're a small team. If something seems stuck, a polite bump on the thread is welcome.

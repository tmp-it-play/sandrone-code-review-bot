# Sandrone Code Review Bot

<div align="center">
  <img width="700" alt="Sandrone Code Review Bot" src="https://github.com/user-attachments/assets/c070f082-f707-41fa-8bfc-769c83dd7e74" />
</div>

Sandrone Code Review Bot is an AI code review bot that summarizes the changes in a GitHub Pull Request and reviews the code to find problems that actually matter.

<sub>Sandrone Code Review Bot is not affiliated with HoYoverse. The trademarks and copyrights for content and materials relating to the Genshin Impact character "Sandrone" belong to HoYoverse.</sub>

[한국어](README.md)

[GitHub App profile](https://github.com/apps/sandrone-code-review-bot) · [Privacy policy](PRIVACY.md)

## Features

- When a PR is opened, it summarizes the changes and lays out the key change in each file.
- It leaves inline comments on problematic code explaining the cause and the impact.
- It reads the repository's `AGENTS.md`, `CLAUDE.md`, contribution guide, and similar files, and applies the project's rules to the review.
- It reviews large PRs in several batches, and tells you when files were left unreviewed.
- It does not repost an issue it has already raised, nor a failure notice for the same job.
- It retries transient external failures with exponential backoff.
- You can request a review, a summary, or a follow-up reply with a command or a bot mention.
- Review language, tone, severity, and target files are configurable per repository.

## Usage

A new PR is reviewed automatically when it is opened, whether or not it is a draft. Automatic review also runs when a draft PR is marked ready for review.

### Commands

The following commands are available in a PR comment.

| Command                     | Action                                               |
|-----------------------------|------------------------------------------------------|
| `/pr-review`                | Reviews the current changes again.                   |
| `/pr-summary`               | Summarizes the changes only, without raising issues. |
| `@sandrone-code-review-bot` | Requests a new review on the PR.                     |

In a review thread, the following commands return an answer based on the latest code.

| Command                     | Action                                                           |
|-----------------------------|------------------------------------------------------------------|
| `/pr-review-reply`          | Checks whether the existing issue has been resolved and replies. |
| `@sandrone-code-review-bot` | Replies in the current review conversation.                      |

You can append instructions after a command that apply only to that request.

```text
/pr-review Focus only on authentication and authorization checks
```

A PR author can run commands on their own PR. Everyone else needs Write, Maintain, or Admin permission on the repository.

## Repository configuration

Review behavior is configured in `.reviewbot/config.yml` in your repository.

```yaml
language: ko
tone: professional
allowStrongTone: false
emoji: false
minSeverity: minor
maxInlineComments: 25

include:
  - "**/*.go"
exclude:
  - "**/generated/**"

sandrone:
  autoReview: true
  autoReviewOnDraft: true
  autoReviewOnPush: false
  progressMessageTheme: programming
  summaryPlacement: new-comment
  maxReviewBatches: 8
```

### Review settings

| Setting             |              Default | Description                                                            |
|---------------------|---------------------:|------------------------------------------------------------------------|
| `language`          |                 `ko` | Language used for the review. `ko`, `en`, and `ja` are available.      |
| `tone`              |       `professional` | Writing style applied to reviews, summaries, and thread replies.       |
| `allowStrongTone`   |              `false` | Allows firm, very direct assessment and criticism of code and PRs.     |
| `emoji`             |              `false` | Prefixes severity badges with a colored circle emoji.                  |
| `minSeverity`       |              `minor` | Minimum severity to post. One of `critical`, `major`, `minor`, `nit`.  |
| `maxInlineComments` |                 `25` | Maximum number of inline comments left in a single review.             |
| `include`           |                  all | Glob patterns for the files to review.                                 |
| `exclude`           | default exclude list | Adds glob patterns for files to exclude from review.                   |
| `threadReply`       |               `true` | Turns follow-up replies in review threads on or off.                   |

Top-level settings are the shared values for every app that uses this file. Writing the same setting under `sandrone:` overrides it for Sandrone only.

```yaml
tone: polite

it-play:
  tone: intelligent

sandrone:
  tone: sandrone
```

In this example Sandrone ignores the other app's `it-play:` settings and uses the Sandrone tone from `sandrone:`. If `sandrone:` has no `tone`, the top-level `polite` applies. Setting precedence is `sandrone:` values first, then top-level shared values, then Sandrone's built-in defaults.

### Sandrone settings

Not only review settings such as `language`, `tone`, `allowStrongTone`, and `minSeverity`, but also the Sandrone behavior settings below can be written as top-level shared values. When the same key exists under `sandrone:`, that value takes precedence; when neither is present, the built-in default is used.

| Setting                |                  Default | Description                                                                                                                                |
|------------------------|-------------------------:|--------------------------------------------------------------------------------------------------------------------------------------------|
| `autoReview`           |                   `true` | Turns automatic review of new PRs on or off.                                                                                               |
| `autoReviewOnDraft`    |                   `true` | Turns automatic review of PRs opened as drafts on or off.                                                                                  |
| `autoReviewOnPush`     |                  `false` | Reviews the changes again when new commits are added.                                                                                      |
| `progressMessageTheme` |            `programming` | The set of progress messages that rotate about every 30 seconds while a review runs. One of `programming`, `sandrone`, `stock`, `history`. |
| `summaryPlacement`     |            `new-comment` | Where the summary goes. One of `new-comment`, `update-comment`, `pr-body`.                                                                 |
| `providers`            |                      all | List of provider names to use for reviews. Leave it empty to allow every available provider.                                               |
| `maxInstructionChars`  |                  `20000` | Total number of characters that can be read from the repository instructions.                                                              |
| `maxReviewBatches`     |                      `8` | Maximum number of batches a large PR is split into for review.                                                                             |
| `instructionFiles`     | default instruction list | List of instruction file paths and globs to read.                                                                                          |

Review settings and instruction files are read from a pinned head SHA of the PR's source branch. For a `develop` → `main` PR, for example, the head commit of `develop` is used. Repository settings cannot lift the cost limits either: review batches are capped at `1`–`8`, model output at `8192` tokens, additional file reads at `6`, and posted findings at `25` in total.

## Tone presets

The following values are available for `tone`.

| Value          | Description                                                                                                                                |
|----------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `professional` | A professional register that delivers conclusions and reasoning calmly and neutrally. This is the default.                                 |
| `intelligent`  | An analytical register that explains premises, causality, and boundary conditions precisely.                                               |
| `polite`       | A courteous register that suggests gently and considerately without blurring its judgment.                                                 |
| `sandrone`     | A casual, contemporary register inspired by "Sandrone" from Genshin Impact: rational, self-assured, and not hiding irritation or cynicism. |

In Korean these presets also select the speech level, with `professional`, `intelligent`, and `polite` using the formal register and `sandrone` using the casual one.

`allowStrongTone: true` allows the bot to assess or criticize the codebase, the PR changes, the design, the implementation, and the behavior firmly and very directly, based on evidence. It does not assess the ability, intent, attitude, or personality of the author or the team, and it does not permit profanity, insults, mockery, threats, or exaggerated severity. It adds no new criticism to responses whose purpose is not assessment, such as summaries.

With `sandrone`, when the strong tone is on, pointed rhetorical questions, dry sarcasm, and brief irritation may appear only in the body of well-founded findings and in replies about unresolved defects. Even then they target only the code and the PR, and are not applied to summaries, finding titles, or code suggestions.

The Korean `sandrone` prompt includes short in-game phrases as a rhythm reference for the speaking style. It is constrained from copying those lines verbatim or pulling the original work's characters and events into a review.

The Sandrone tone is used only in repositories that select it explicitly, like this:

```yaml
language: ko
tone: sandrone
allowStrongTone: true
```

`sandrone` is not the default; without configuration the `professional` tone is used.


## License

[MIT](LICENSE)

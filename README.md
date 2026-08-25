# Sandrone Code Review Bot

<div align="center">
  <img width="700" alt="Codex 이미지 2026년 8월 23일 오전 04_37_56" src="https://github.com/user-attachments/assets/c070f082-f707-41fa-8bfc-769c83dd7e74" />
</div>

Sandrone Code Review Bot은 GitHub Pull Request의 변경 내용을 요약하고, 실제로 문제가 되는 코드를 찾아 리뷰하는 AI 코드 리뷰 봇입니다.

<sub>Sandrone Code Review Bot는 HoYoverse와 연관이 없습니다.Genshin Impact의 캐릭터 「산드로네」에 대한 콘텐츠와 소재의 트레이드마크와 저작권은 HoYoverse에 있습니다.</sub>

## 주요 기능

- PR이 열리면 변경 내용을 요약하고 파일별 핵심 변경 사항을 정리합니다.
- 문제가 있는 코드에는 원인과 영향을 설명하는 인라인 코멘트를 남깁니다.
- 저장소의 `AGENTS.md`, `CLAUDE.md`, 기여 가이드 등을 읽고 프로젝트 규칙을 리뷰에 반영합니다.
- 큰 PR도 여러 묶음으로 나누어 검토하고, 검토하지 못한 파일이 있으면 알려 줍니다.
- 이미 지적한 문제는 다시 게시하지 않습니다.
- 명령이나 봇 멘션으로 리뷰, 요약, 후속 답변을 요청할 수 있습니다.
- 저장소마다 리뷰 언어, 문체, 심각도, 대상 파일을 설정할 수 있습니다.

## 사용법

새 PR이 열리거나 Draft PR이 리뷰 준비 상태로 바뀌면 자동 리뷰가 실행됩니다. 기본 설정에서는 이후 커밋이 추가되어도 자동으로 다시 리뷰하지 않습니다. 다만 자동 리뷰가 활성화된 상태에서 진행 중인 리뷰의 head가 변경되면 오래된 실행을 중단하고 최신 head에서 대체 리뷰를 시작합니다.

### 명령

PR 코멘트에서 다음 명령을 사용할 수 있습니다.

| 명령                        | 동작                                   |
|-----------------------------|----------------------------------------|
| `/pr-review`                | 현재 변경 사항을 다시 리뷰합니다.      |
| `/pr-summary`               | 문제 지적 없이 변경 내용만 요약합니다. |
| `@sandrone-code-review-bot` | PR에서 새 리뷰를 요청합니다.           |

리뷰 스레드에서는 다음 명령으로 최신 코드를 확인한 답변을 받을 수 있습니다.

| 명령                        | 동작                                        |
|-----------------------------|---------------------------------------------|
| `/pr-review-reply`          | 기존 지적이 해결되었는지 확인하고 답합니다. |
| `@sandrone-code-review-bot` | 현재 리뷰 대화에 답합니다.                  |

명령 뒤에는 이번 요청에만 적용할 지시를 덧붙일 수 있습니다.

```text
/pr-review 인증과 권한 검사만 집중해서 봐줘
```

PR 작성자는 자신의 PR에서 명령을 실행할 수 있습니다. 그 외 사용자는 저장소의 Write, Maintain 또는 Admin 권한이 필요합니다.

## 저장소 설정

저장소의 `.reviewbot/config.yml`에서 리뷰 동작을 설정합니다.

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
  autoReviewOnPush: false
  summaryPlacement: new-comment
  maxReviewBatches: 8
```

### 리뷰 설정

| 설정                  |         기본값 | 설명                                                                         |
|-----------------------|---------------:|------------------------------------------------------------------------------|
| `language`            |           `ko` | 리뷰에 사용할 언어입니다. `ko`, `en`, `ja`를 사용할 수 있습니다.             |
| `tone`                | `professional` | 리뷰, 요약, 스레드 답글에 적용할 문체입니다.                                 |
| `allowStrongTone`     |        `false` | 코드와 PR을 단호하고 매우 직설적으로 평가하거나 비판하도록 허용합니다.       |
| `emoji`               |        `false` | 심각도 배지 앞에 색 원 이모지를 붙입니다.                                    |
| `minSeverity`         |        `minor` | 게시할 최소 심각도입니다. `critical`, `major`, `minor`, `nit` 중 하나입니다. |
| `maxInlineComments`   |           `25` | 한 번의 리뷰에서 남길 최대 인라인 코멘트 수입니다.                           |
| `include`             |           전체 | 리뷰할 파일의 glob 패턴입니다.                                               |
| `exclude`             | 기본 제외 목록 | 리뷰에서 제외할 파일의 glob 패턴을 추가합니다.                               |
| `threadReply`         |         `true` | 리뷰 스레드의 후속 답변을 켜거나 끕니다.                                     |

최상위 리뷰 설정은 이 파일을 함께 사용하는 앱의 공통값입니다. `sandrone:` 아래에 같은 설정을 작성하면 Sandrone에서만 그 값으로 덮어씁니다.

```yaml
tone: polite

it-play:
  tone: intelligent

sandrone:
  tone: sandrone
```

이 예시에서 Sandrone은 다른 앱의 `it-play:` 설정을 무시하고 `sandrone:`의 산드로네 문체를 사용합니다. `sandrone:`에 `tone`이 없다면 최상위의 `polite`를 사용합니다. 설정 우선순위는 `sandrone:` 전용값, 최상위 공통값, Sandrone 기본값 순서입니다.

### Sandrone 설정

`language`, `tone`, `allowStrongTone`, `minSeverity` 같은 리뷰 설정은 모두 `sandrone:` 아래에서 Sandrone 전용값으로 다시 지정할 수 있습니다.

| 설정                        |        기본값 | 설명                                                                       |
|-----------------------------|--------------:|----------------------------------------------------------------------------|
| `sandrone.autoReview`       |        `true` | 새 PR의 자동 리뷰를 켜거나 끕니다.                                         |
| `sandrone.autoReviewOnPush` |       `false` | 새 커밋이 추가될 때 변경분을 다시 리뷰합니다. 진행 중인 리뷰의 head 변경은 이 설정과 무관하게 대체 실행됩니다. |
| `sandrone.summaryPlacement` | `new-comment` | 요약 위치입니다. `new-comment`, `update-comment`, `pr-body` 중 하나입니다. |
| `sandrone.maxReviewBatches` |           `8` | 큰 PR을 의미 단위로 나누어 검토할 최대 횟수입니다.                          |

리뷰 설정과 지침 파일은 PR의 변경 브랜치가 아니라 고정된 base SHA에서 읽습니다. 저장소 설정으로도 비용 상한을 해제할 수 없으며 리뷰 배치는 `1~8`, 모델 출력은 최대 `8192` token, 추가 파일 읽기는 최대 `6`, 게시 finding은 전체 `25`건으로 제한됩니다.

## 문체 프리셋

`tone`에는 다음 값을 사용할 수 있습니다.

| 값             | 설명                                                                                                     |
|----------------|----------------------------------------------------------------------------------------------------------|
| `professional` | 결론과 근거를 차분하고 중립적으로 전달하는 전문적인 존댓말입니다. 기본값입니다.                          |
| `intelligent`  | 전제, 인과관계, 경계 조건을 정밀하게 설명하는 분석적인 존댓말입니다.                                     |
| `polite`       | 판단을 흐리지 않으면서 배려 깊고 부드럽게 제안하는 정중한 존댓말입니다.                                  |
| `sandrone`     | 원신의 「산드로네」에게서 영감을 받은 이성적이고 자신만만하며 짜증과 냉소도 숨기지 않는 현대 반말입니다. |

`allowStrongTone: true`는 코드베이스, PR 변경, 설계, 구현, 동작을 근거에 따라 단호하고 매우 직설적으로 평가하거나 비판하도록 허용합니다. 작성자나 팀의 능력, 의도, 태도, 성격은 평가하지 않으며 욕설, 모욕, 조롱, 위협과 심각도 과장도 허용하지 않습니다. 요약처럼 평가가 목적이 아닌 응답에는 새로운 비판을 추가하지 않습니다.

`sandrone`에서는 강한 어조가 켜지면 근거가 충분한 finding 본문과 미해결 결함에 대한 답글에 한해 날 선 반문, 건조한 빈정거림, 짧은 짜증을 사용할 수 있습니다. 이런 표현의 대상도 코드와 PR로 한정되며 요약, finding 제목, 코드 suggestion에는 적용하지 않습니다.

한국어 `sandrone` 프롬프트에는 짧은 인게임 표현을 화법의 리듬 참고로 포함합니다. 해당 문장을 그대로 복사하거나 원작의 인물과 사건을 리뷰에 끌어오지는 않도록 제한합니다.

산드로네 문체는 다음처럼 직접 선택한 저장소에서만 사용됩니다.

```yaml
language: ko
tone: sandrone
allowStrongTone: true
```

`sandrone`은 기본값이 아니며, 설정이 없으면 `professional` 문체를 사용합니다.


## 라이선스

[MIT](LICENSE)

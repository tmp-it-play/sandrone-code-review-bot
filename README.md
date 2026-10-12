# Sandrone Code Review Bot

<div align="center">
  <img width="700" alt="Codex 이미지 2026년 8월 23일 오전 04_37_56" src="https://github.com/user-attachments/assets/c070f082-f707-41fa-8bfc-769c83dd7e74" />
</div>

Sandrone Code Review Bot은 GitHub Pull Request의 변경 내용을 요약하고, 실제로 문제가 되는 코드를 찾아 리뷰하는 AI 코드 리뷰 봇입니다.

<sub>Sandrone Code Review Bot는 HoYoverse와 연관이 없습니다.Genshin Impact의 캐릭터 「산드로네」에 대한 콘텐츠와 소재의 트레이드마크와 저작권은 HoYoverse에 있습니다.</sub>

[English](README.en.md)

[GitHub App 프로필](https://github.com/apps/sandrone-code-review) · [개인정보처리방침](PRIVACY.md)

## 주요 기능

- PR이 열리면 변경 내용을 요약하고 파일별 핵심 변경 사항을 정리합니다.
- 문제가 있는 코드에는 원인과 영향을 설명하는 인라인 코멘트를 남깁니다.
- 저장소의 `AGENTS.md`, `CLAUDE.md`, 기여 가이드 등을 읽고 프로젝트 규칙을 리뷰에 반영합니다.
- 큰 PR도 여러 묶음으로 나누어 검토하고, 검토하지 못한 파일이 있으면 알려 줍니다.
- 이미 지적한 문제와 같은 작업의 실패 안내는 다시 게시하지 않습니다.
- 일시적인 외부 장애는 지수 백오프로 재시도합니다.
- 명령이나 봇 멘션으로 리뷰, 요약, 후속 답변을 요청할 수 있습니다.
- 저장소마다 리뷰 언어, 문체, 심각도, 대상 파일을 설정할 수 있습니다.

## 사용법

새 PR은 Draft 여부와 관계없이 열릴 때 자동 리뷰가 실행됩니다. Draft PR이 리뷰 준비 상태로 바뀐 때의 자동 리뷰도 실행됩니다.

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
  autoReviewOnDraft: true
  autoReviewOnPush: false
  progressMessageTheme: programming
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

최상위 설정은 이 파일을 함께 사용하는 앱의 공통값입니다. `sandrone:` 아래에 같은 설정을 작성하면 Sandrone에서만 그 값으로 덮어씁니다.

```yaml
tone: polite

it-play:
  tone: intelligent

sandrone:
  tone: sandrone
```

이 예시에서 Sandrone은 다른 앱의 `it-play:` 설정을 무시하고 `sandrone:`의 산드로네 문체를 사용합니다. `sandrone:`에 `tone`이 없다면 최상위의 `polite`를 사용합니다. 설정 우선순위는 `sandrone:` 전용값, 최상위 공통값, Sandrone 기본값 순서입니다.

### Sandrone 설정

`language`, `tone`, `allowStrongTone`, `minSeverity` 같은 리뷰 설정뿐 아니라 아래의 Sandrone 동작 설정도 최상위 공통값으로 작성할 수 있습니다. `sandrone:` 아래에 같은 키가 있으면 그 값을 우선하고, 둘 다 없으면 내장 기본값을 사용합니다.

| 설정                   |         기본값 | 설명                                                                                                               |
|------------------------|---------------:|--------------------------------------------------------------------------------------------------------------------|
| `autoReview`           |         `true` | 새 PR의 자동 리뷰를 켜거나 끕니다.                                                                                 |
| `autoReviewOnDraft`    |         `true` | Draft 상태로 열린 PR의 자동 리뷰를 켜거나 끕니다.                                                                  |
| `autoReviewOnPush`     |        `false` | 새 커밋이 추가될 때 변경분을 다시 리뷰합니다.                                                                      |
| `progressMessageTheme` |  `programming` | 리뷰 처리 중 약 30초마다 바뀌는 진행 문구 묶음입니다. `programming`, `sandrone`, `stock`, `history` 중 하나입니다. |
| `summaryPlacement`     |  `new-comment` | 요약 위치입니다. `new-comment`, `update-comment`, `pr-body` 중 하나입니다.                                         |
| `providers`            |           전체 | 리뷰에 사용할 provider 이름 목록입니다. 비워 두면 사용 가능한 provider를 모두 허용합니다.                          |
| `maxInstructionChars`  |        `20000` | 저장소 지침에서 읽을 수 있는 전체 문자 수입니다.                                                                   |
| `maxReviewBatches`     |            `8` | 큰 PR을 의미 단위로 나누어 검토할 최대 횟수입니다.                                                                 |
| `instructionFiles`     | 기본 지침 목록 | 읽을 지침 파일 경로와 glob 목록입니다.                                                                             |

리뷰 설정과 지침 파일은 PR 출발지 브랜치의 고정된 head SHA에서 읽습니다. 예를 들어 `develop` → `main` PR이면 `develop`의 head 커밋을 기준으로 합니다. 저장소 설정으로도 비용 상한을 해제할 수 없으며 초기 리뷰 배치는 `1~8`, 모델 출력 설정은 최대 `8192` token, 추가 파일 읽기는 최대 `6`, 게시 finding은 전체 `25`건으로 제한됩니다. 실제 용량·응답 형식 문제는 호출 예산 안에서 추가 분할합니다. 일시 장애나 쿨다운은 분할 사유로 사용하지 않습니다.

운영 환경의 `SANDRONE_REQUEST_TIMEOUT`은 외부 모델 요청 한 건의 제한 시간입니다. 기본값과 상한은 `3m`입니다. `SANDRONE_REVIEW_TIMEOUT`은 리뷰 실행 시작부터 재시도 대기까지 포함한 모델 처리 시간 예산입니다. 기본값은 `30m`, 상한은 `2h`입니다. 시간 예산을 소진하면 새 모델 호출을 중단하고 확보한 결과와 미검토 범위를 정리합니다. 결과 저장과 게시에는 별도 시간이 필요합니다. `SANDRONE_LLM_MAX_CALLS_PER_OPERATION`의 전체 호출 상한은 `24`회입니다.

프로바이더는 Groq, Gemini, OpenRouter, NVIDIA, Local, Mistral, Cloudflare 순서로 시도합니다. Local은 요약 전용입니다. Groq는 `openai/gpt-oss-120b`, Gemini는 `gemini-3.8-flash`, OpenRouter는 `nvidia/nemotron-3-super-120b-a12b:free`를 사용합니다. Groq와 OpenRouter의 리뷰 결과에는 JSON Schema를 적용합니다. Groq는 구조화 응답과 도구 호출의 병용을 지원하지 않으므로 추가 도구 호출을 사용하지 않습니다. OpenRouter는 출력 토큰이 추론에 모두 소비되는 실패를 막기 위해 추론을 끄고, 구조화 응답을 지원하는 경로만 요청합니다. 응답은 요약과 지적 배열의 존재 여부도 검증합니다.

리뷰 배치는 현재 쿨다운이 아닌 프로바이더 중 작은 입력·출력 용량을 우선합니다. 초기 배치 수가 설정 상한을 넘으면 큰 용량으로 계획하고, 실행 중 호출 예산 안에서 다시 분할합니다. 단일 파일의 필수 diff가 작은 프로바이더에 맞지 않으면 큰 프로바이더로 넘깁니다. 선택적 문맥은 줄일 수 있지만 필수 diff를 잘라서 검토 완료로 처리하지 않습니다. 다른 프로바이더의 쿨다운은 사용 가능한 프로바이더에서 발생한 용량·응답 형식 문제의 분할을 막지 않습니다.

같은 실행에서 완료한 검토 단위는 재시도 시 입력 hash가 달라져도 저장 결과를 보존합니다. 이미 지난 재시도 시각에는 추가 지수 백오프를 적용하지 않습니다. 프로바이더가 지정한 `Retry-After`를 우선하며 상한은 24시간입니다. 지정값이 없는 일시적인 호출 제한·서비스 장애·시간 초과의 쿨다운은 최대 1분입니다. 인증·모델·할당량 오류에는 설정된 쿨다운을 적용합니다. 긴 요청의 실패는 같은 프로바이더로 즉시 재시도하지 않습니다. 대시보드는 앱 내부 호출 예산·예약 오류를 외부 프로바이더 실패와 별도로 집계합니다.

실행 lease가 만료되고 진행 기록이 `max(3h, 2 × SANDRONE_REVIEW_TIMEOUT)` 동안 갱신되지 않은 미완료 실행은 5분 간격으로 정리합니다. 게시 중인 실행과 유효한 lease는 이 정리에서 제외합니다. 성공한 검토 결과는 보존하고 미완료 범위를 실패로 기록합니다. 웹훅 선점 트랜잭션은 `READ COMMITTED`와 `FOR UPDATE SKIP LOCKED`를 사용해 동시 작업자의 범위 잠금 충돌을 줄입니다.

### 무료 프로바이더 정책 확인

확인일은 2026-10-12입니다. 저장소 최초 생성일은 2026-08-21이며 LLM 연동은 2026-08-23에 시작했습니다. 아래 내용은 공식 문서 기준입니다. 계정별 실제 한도와 남은 용량은 각 관리 화면 또는 API에서 확인해야 합니다.

| 서비스 | 현재 조건과 개발 이후 변경 | 운영 판단 |
|--------|----------------------------|-----------|
| Gemini | 2026-10-08에 3.7 Flash가 deprecated 처리되어 3.8 Flash로 자동 연결됩니다. 3.8 Flash의 무료 입력·출력은 유지됩니다. 제한은 프로젝트 단위입니다. [변경 이력](https://ai.google.dev/gemini-api/docs/changelog), [요금](https://ai.google.dev/gemini-api/docs/pricing), [한도](https://ai.google.dev/gemini-api/docs/rate-limits) | 모델명을 3.8로 갱신했습니다. 최근 429만으로 계정 한도 축소를 단정하지 않습니다. |
| Groq | GPT-OSS 120B 무료 공개 한도는 30 RPM, 1,000 RPD, 8,000 TPM, 200,000 TPD입니다. 9월에는 Qwen 3.6과 Compound 계열 변경이 있었지만 현재 사용하는 GPT-OSS 120B 종료 공지는 없습니다. [한도](https://console.groq.com/docs/rate-limits), [종료 이력](https://console.groq.com/docs/deprecations), [구조화 응답](https://console.groq.com/docs/structured-outputs) | 입력 크기와 출력 예산을 맞추고 우선 사용합니다. 다른 Groq 모델도 계정 용량을 고려해야 합니다. |
| OpenRouter | 무료 모델에도 일일 호출 한도가 있습니다. `GET /api/v1/key`의 `free_model_daily_requests`로 계정의 사용량·상한·잔량을 확인할 수 있습니다. [한도](https://openrouter.ai/docs/api/reference/limits) | 모델을 추가해도 플랫폼 무료 호출 한도가 독립적으로 늘어나지는 않습니다. |
| Cloudflare | 무료 Workers AI 할당은 하루 10,000 Neurons이며 00:00 UTC에 초기화됩니다. 일부 최신 대형 모델은 유료 설정이 필요합니다. [요금](https://developers.cloudflare.com/workers-ai/platform/pricing/) | GLM과 Gemma 경로는 같은 계정 할당량을 공유합니다. 새 모델을 모두 무료로 간주하지 않습니다. |
| Mistral | Free 모드의 월간 포함 사용량과 모델별 처리량 제한을 적용합니다. 실제 한도는 관리 화면에 표시됩니다. [한도](https://docs.mistral.ai/admin/billing-usage/usage-limits) | 오류 코드만으로 분당 제한과 월간 소진을 구분하지 않습니다. 계정 정책 변경 시점은 확인되지 않았습니다. |
| Cerebras | 현재 신규 계정 혜택은 결제 수단 확인 후 지급되는 5달러·30일 체험 크레딧입니다. [한도](https://inference-docs.cerebras.ai/support/rate-limits) | 지속적으로 갱신되는 무료 대안으로 추가하지 않습니다. 전환 날짜는 확인되지 않았습니다. |
| Hugging Face Inference Providers | 현재 무료 사용자에게 포함된 월간 크레딧은 없습니다. [요금](https://huggingface.co/docs/inference-providers/pricing) | 과거 무료 크레딧 안내를 근거로 대체 경로에 추가하지 않습니다. 변경 날짜는 확인되지 않았습니다. |
| GitHub Models | 2026-07-30에 서비스가 종료되었습니다. [공식 안내](https://docs.github.com/en/github-models) | 저장소 개발 시작 전 변경입니다. 현재 대체 공급자로 사용할 수 없습니다. |

추가 평가 후보는 OpenRouter의 [Apodex 1.1 Mini 무료 모델](https://openrouter.ai/apodex/apodex-1.1-mini:free)입니다. 2026-10-01 공개 모델이며 구조화 응답과 도구 호출을 지원합니다. 다만 기존 OpenRouter와 무료 호출 한도를 공유하고 리뷰 품질을 검증하지 않았으므로 기본 경로에는 추가하지 않았습니다. Groq의 [GPT-OSS 20B](https://console.groq.com/docs/models)도 구조화 응답을 지원하는 같은 공급자 내 후보입니다. 두 후보 모두 독립된 무료 공급자 용량을 확보하는 대안은 아닙니다.

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

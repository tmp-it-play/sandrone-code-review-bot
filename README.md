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

새 PR이 열리거나 Draft PR이 리뷰 준비 상태로 바뀌면 자동 리뷰가 실행됩니다. 기본 설정에서는 이후 커밋이 추가되어도 자동으로 다시 리뷰하지 않습니다.

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

| 설정                |         기본값 | 설명                                                                         |
|---------------------|---------------:|------------------------------------------------------------------------------|
| `language`          |           `ko` | 리뷰에 사용할 언어입니다. `ko`, `en`, `ja`를 사용할 수 있습니다.             |
| `tone`              | `professional` | 리뷰, 요약, 스레드 답글에 적용할 문체입니다.                                 |
| `emoji`             |        `false` | 심각도 배지 앞에 색 원 이모지를 붙입니다.                                    |
| `minSeverity`       |        `minor` | 게시할 최소 심각도입니다. `critical`, `major`, `minor`, `nit` 중 하나입니다. |
| `maxInlineComments` |           `25` | 한 번의 리뷰에서 남길 최대 인라인 코멘트 수입니다.                           |
| `include`           |           전체 | 리뷰할 파일의 glob 패턴입니다.                                               |
| `exclude`           | 기본 제외 목록 | 리뷰에서 제외할 파일의 glob 패턴을 추가합니다.                               |
| `threadReply`       |         `true` | 리뷰 스레드의 후속 답변을 켜거나 끕니다.                                     |

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

`language`, `tone`, `minSeverity` 같은 리뷰 설정은 모두 `sandrone:` 아래에서 Sandrone 전용값으로 다시 지정할 수 있습니다.

| 설정                        |        기본값 | 설명                                                                       |
|-----------------------------|--------------:|----------------------------------------------------------------------------|
| `sandrone.autoReview`       |        `true` | 새 PR의 자동 리뷰를 켜거나 끕니다.                                         |
| `sandrone.autoReviewOnPush` |       `false` | 새 커밋이 추가될 때 변경분을 다시 리뷰합니다.                              |
| `sandrone.summaryPlacement` | `new-comment` | 요약 위치입니다. `new-comment`, `update-comment`, `pr-body` 중 하나입니다. |
| `sandrone.maxReviewBatches` |           `8` | 큰 PR을 의미 단위로 나누어 검토할 최대 횟수입니다.                          |

리뷰 설정과 지침 파일은 PR의 변경 브랜치가 아니라 고정된 base SHA에서 읽습니다. 저장소 설정으로도 비용 상한을 해제할 수 없으며 리뷰 배치는 `1~8`, 모델 출력은 최대 `8192` token, 추가 파일 읽기는 최대 `6`, 게시 finding은 전체 `25`건으로 제한됩니다.

## 문체 프리셋

`tone`에는 다음 값을 사용할 수 있습니다.

| 값             | 설명                                                                               |
|----------------|------------------------------------------------------------------------------------|
| `professional` | 결론과 근거를 차분하고 중립적으로 전달하는 전문적인 존댓말입니다. 기본값입니다.    |
| `intelligent`  | 전제, 인과관계, 경계 조건을 정밀하게 설명하는 분석적인 존댓말입니다.               |
| `polite`       | 판단을 흐리지 않으면서 배려 깊고 부드럽게 제안하는 정중한 존댓말입니다.            |
| `sandrone`     | 원신의 산드로네에게서 영감을 받은 이성적이고 자신감 있으며 도도한 현대 반말입니다. |

산드로네 문체는 다음처럼 직접 선택한 저장소에서만 사용됩니다.

```yaml
language: ko
tone: sandrone
```

`sandrone`은 기본값이 아니며, 설정이 없으면 `professional` 문체를 사용합니다.

## 운영 설정

애플리케이션은 MySQL 8과 Redis가 필요합니다. 환경 변수 예시는 [`.env.example`](.env.example)에 있습니다. GitHub App의 webhook URL은 기본 경로에서는 `/webhook`, `SANDRONE_BASE_PATH=/sandrone`이면 `/sandrone/webhook`입니다.

### 필수 런타임 환경 변수

| 변수 | 설명 |
|------|------|
| `GITHUB_APP_ID` | GitHub App ID |
| `GITHUB_PRIVATE_KEY` 또는 `GITHUB_PRIVATE_KEY_PATH` | PEM 개인 키 본문 또는 개인 키 파일 경로 |
| `GITHUB_WEBHOOK_SECRET` | GitHub App webhook secret |
| `MYSQL_DSN` | MySQL 8 접속 DSN |
| `DASHBOARD_USERNAME` | 운영 대시보드 사용자 이름 |
| `DASHBOARD_PASSWORD` | 운영 대시보드 비밀번호 |
| `DASHBOARD_SESSION_SECRET` | 대시보드 세션 서명용 난수 secret |
| LLM provider credentials | 아래 정식 provider의 credential 전체 |

### LLM 프로바이더

| 프로바이더 이름 | 환경 변수 | 활성화 |
|-----------------|-----------|--------|
| `gemini` | `GEMINI_API_KEY` | 정식 사용 |
| `groq` | `GROQ_API_KEY` | 정식 사용 |
| `openrouter` | `OPENROUTER_API_KEY` | 정식 사용 |
| `nvidia` | `NVIDIA_API_KEY` | 정식 사용 |
| `mistral` | `MISTRAL_API_KEY` | 정식 사용 |
| `cloudflare-glm`, `cloudflare-gemma` | `CLOUDFLARE_API_TOKEN`과 `CLOUDFLARE_ACCOUNT_ID` 모두 | 정식 사용 |

공개 저장소는 활성화된 프로바이더를 사용할 수 있습니다. 비공개 저장소의 코드는 기본적으로 외부 프로바이더에 전송하지 않습니다. 운영자가 데이터 처리 조건을 확인하고 승인한 프로바이더 이름만 `SANDRONE_PRIVATE_CODE_PROVIDERS`에 쉼표로 지정해야 합니다. 비공개 코드 허용 목록이 비어 있으면 리뷰 요청은 외부 전송 없이 실패합니다.

Mistral key는 [Mistral Studio API key 안내](https://docs.mistral.ai/getting-started/quickstarts/studio/activate-and-generate-api-key)에서, Cloudflare token과 Account ID는 [Workers AI REST API 안내](https://developers.cloudflare.com/workers-ai/get-started/rest-api/)에서 발급합니다.

Mistral Free mode는 [기본 privacy 설정에서 API 입력과 출력을 모델 개선에 사용할 수 있으므로](https://help.mistral.ai/en/articles/455207-can-i-opt-out-of-my-input-or-output-data-being-used-for-training), 비공개 코드 허용 목록에 `mistral`을 넣기 전에 Mistral Admin의 Privacy 설정에서 학습 사용을 꺼야 합니다. 현재 Cloudflare 연결은 AI Gateway logging을 거치지 않는 Workers AI direct endpoint를 사용합니다.

`SANDRONE_LLM_MAX_CALLS_PER_OPERATION`은 한 리뷰·요약·답변 작업이 사용할 수 있는 외부 LLM 호출과 모델이 요청한 도구 실행의 공통 상한입니다. 기본값과 절대 상한은 `12`이며 허용 범위는 `1`부터 `12`까지입니다. 프로바이더를 여러 개 설정해도 이 공통 예산을 넘지 않습니다.

### 런타임 기본값

| 변수 | 기본값 또는 동작 |
|------|------------------|
| `SANDRONE_HTTP_ADDR` | `:8080` |
| `SANDRONE_BASE_PATH` | 빈 값 |
| `SANDRONE_LOG_LEVEL` | `info`; `debug`, `warn`, `error` 지원 |
| `REDIS_ADDR` | `127.0.0.1:6379` |
| `REDIS_PASSWORD` | 빈 값 |
| `REDIS_DB` | `0` |
| `SANDRONE_WORKER_CONCURRENCY` | 기본 `2`; CD도 `2`로 고정 |
| `SANDRONE_PROVIDER_COOLDOWN` | `15m` |
| `SANDRONE_REQUEST_TIMEOUT` | `5m` |
| `SANDRONE_LLM_MAX_CALLS_PER_OPERATION` | 기본 `12`; `1`부터 `12`까지 적용 |
| `SANDRONE_PRIVATE_CODE_PROVIDERS` | 비공개 코드 전송을 검토·승인한 프로바이더 이름 목록; 기본은 빈 값 |
| `SANDRONE_DELIVERY_RETENTION` | 최소 `96h` |
| `SANDRONE_REVIEW_RETENTION` | 기본 `4320h`; `2880h`부터 `4320h`까지 적용 |
| `SANDRONE_METRICS_TOKEN` | 설정하면 `/metrics`에 Bearer token 인증 적용; 운영 환경에서는 설정 권장 |
| `SANDRONE_TEMPLATE_DIR` | `web/template` |
| `SANDRONE_STATIC_DIR` | `web/static` |

### GitHub Actions CD

`main`의 코드, 워크플로 또는 배포 파일이 바뀌면 CD가 한 번 실행됩니다. 먼저 formatting, `go test`, `go vet`, 실행 파일 build를 통과해야 하고, 그 뒤 ARM64 이미지를 빌드해 GHCR에 올립니다. 문서와 저장소 메타데이터만 바뀐 push는 배포하지 않습니다.

필수 GitHub Actions secret은 다음과 같습니다.

| Secret | 용도 |
|--------|------|
| `SANDRONE_APP_ID` | 런타임 `GITHUB_APP_ID` |
| `SANDRONE_APP_PRIVATE_KEY` | 런타임 `GITHUB_PRIVATE_KEY` |
| `SANDRONE_WEBHOOK_SECRET` | 런타임 `GITHUB_WEBHOOK_SECRET` |
| `SANDRONE_MYSQL_DSN` | 런타임 `MYSQL_DSN` |
| `DEPLOY_HOST` | 배포 서버 주소 |
| `DEPLOY_USER` | 배포 서버 SSH 사용자 |
| `DEPLOY_PASSWORD` | 배포 서버 SSH 비밀번호 |
| `DASHBOARD_PASSWORD` | 대시보드 비밀번호 |
| `DASHBOARD_SESSION_SECRET` | 대시보드 세션 서명 secret |
| `GEMINI_API_KEY` | Gemini API credential |
| `GROQ_API_KEY` | Groq API credential |
| `OPENROUTER_API_KEY` | OpenRouter API credential |
| `NVIDIA_API_KEY` | NVIDIA API credential |
| `MISTRAL_API_KEY` | Mistral API credential |
| `CLOUDFLARE_API_TOKEN` | Cloudflare Workers AI API credential |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare Workers AI account ID |

CD는 표의 모든 provider credential을 필수로 검사하고 실험 allowlist 없이 정식 provider로 전달합니다. `SANDRONE_REDIS_PASSWORD`와 `SANDRONE_METRICS_TOKEN`은 저장소에 설정된 값을 전달하고, repository variable `SANDRONE_PRIVATE_CODE_PROVIDERS`는 비공개 코드 전송을 승인한 프로바이더만 담습니다. Worker concurrency는 `2`, provider cooldown은 `15m`, 요청 timeout은 `5m`, run당 LLM 호출 상한은 `12`, review 보존 기간은 `4320h`, delivery 보존 기간은 `96h`로 CD가 고정합니다.

배포 서버에는 ARM64 Docker와 GNU `timeout`, 외부 Docker network `readygsm-net`이 있어야 하며, 그 network에서 `redis:6379`와 `SANDRONE_MYSQL_DSN`의 MySQL에 접근할 수 있어야 합니다. CD는 secret을 포함한 배포 묶음을 원격 archive로 저장하지 않고 SSH stdin으로 mode `0700`의 고정 임시 경로에 직접 풉니다. 원격 wrapper는 성공, 실패, 신호 종료 모두에서 해당 경로를 제거하고 env 파일은 존재하는 동안 mode `0600`으로 제한합니다. 새 컨테이너를 시작하기 전에는 현재 컨테이너에 SIGTERM을 보내 HTTP 30초와 Asynq 120초 종료 유예를 포함하는 최대 180초 동안 정상 종료를 기다린 뒤 제거합니다. 이어 main commit SHA 이미지를 실행하고 health check를 통과하면 배포를 완료합니다. 검증에 실패하면 최근 로그를 남기고 실패한 컨테이너를 제거한 뒤 CD를 실패로 끝냅니다. 어떤 컨테이너도 사용하지 않는 같은 GHCR 저장소의 과거 40자리 commit SHA 이미지만 제거합니다. 필수 secret이 누락된 경우에도 배포를 건너뛴 성공 상태로 만들지 않고 사전 점검에서 실패합니다.

단일 host port를 교체하는 동안 GitHub webhook ingress에 짧은 공백이 생길 수 있습니다. 새 애플리케이션은 HTTP listener가 열릴 여유를 둔 뒤 App JWT로 GitHub App delivery API의 최근 30분만 조회하고, 실패 delivery를 한 pass에 최대 10건씩 1초 간격으로 재전송합니다. 이 pass는 시작 후와 5분마다 실행됩니다. 매 pass는 최신 3 page를 다시 확인하면서 이전 pass의 cursor부터 최대 3 page를 추가로 진행하고, cutoff에 도달하면 그 pass의 최신 cursor로 되감습니다. 같은 GUID의 성공 시도와 10분 이내 반복 요청은 제외합니다. Delivery GUID 기반 inbox와 run 식별자가 중복 redelivery를 흡수하므로 전체 delivery 이력을 무제한 재생하지 않습니다. 30분보다 긴 ingress 장애는 GitHub App delivery 화면에서 운영자가 범위를 확인한 뒤 수동 재전송합니다.

## 라이선스

[MIT](LICENSE)

# sandrone-code-review-bot

GitHub Pull Request를 자동으로 리뷰하는 봇입니다. Go로 작성했고 컨테이너 하나로 동작합니다.

## 기능

- PR이 열리면 요약을 일반 코멘트로 남기고 이어서 인라인 코멘트를 답니다
- 이후 푸시는 자동으로 다시 리뷰하지 않습니다 (`sandrone.autoReviewOnPush: true`로 켤 수 있습니다)
- `/pr-review`, `/pr-summary`, 리뷰 스레드에서 `/pr-review-reply` — `@sandrone-code-review-bot` 멘션으로도 호출합니다
- 여러 무료 LLM 프로바이더를 우선순위대로 시도하고, 한도에 걸리면 다음으로 넘어갑니다
- 큰 PR은 파일 단위로 나눠 여러 번 검토하고, 그래도 담지 못한 파일은 요약 아래 토글에 표로 밝힙니다
- 인라인 코멘트를 달지 못한 지적은 요약 코멘트 아래 토글로 합쳐 보여줍니다
- 모든 코멘트 맨 아래에 사용한 프로바이더·모델과 토큰 사용량을 작게 표기합니다

## 요구사항

- Go 1.26+
- MySQL, Redis
- GitHub App (Pull requests RW, Contents R, Metadata R, Issues RW / 이벤트: pull_request, issue_comment, pull_request_review_comment, installation)

## 실행

```bash
cp .env.example .env
go run ./cmd/sandrone
```

Docker로 띄울 때:

```bash
docker compose up -d --build
```

- 웹훅 수신: `POST /webhook`
- 상태 확인: `GET /healthz`
- 지표: `GET /metrics`
- 관리 콘솔: `GET /dashboard`

리버스 프록시 뒤 하위 경로에 붙일 때는 `SANDRONE_BASE_PATH=/sandrone`처럼 지정합니다. 모든 경로와 화면 링크가 그 아래로 이동합니다.

## 배포

`main`에 푸시하면 GitHub Actions가 `linux/arm64` 이미지를 빌드해 `ghcr.io`에 올리고, SSH로 서버에 배포한 뒤 ghcr 패키지를 지웁니다. 서버에는 실행 중인 컨테이너와 이미지만 남고 배포 스크립트는 스스로 정리합니다.

배포 흐름은 `deploy/deployspec.yml`에 정의되어 있습니다.

| 단계 | 하는 일 |
|---|---|
| AfterInstall | ghcr 로그인 → 이미지 pull → 로그아웃 |
| ApplicationStart | 이전 컨테이너 제거 → 새 컨테이너 실행 |
| ValidateService | `/healthz` 확인 → 실패 시 롤백 정리 → 배포 파일 삭제 |

### 필요한 시크릿

| 이름 | 설명 |
|---|---|
| `SANDRONE_APP_ID` | GitHub App ID |
| `SANDRONE_APP_PRIVATE_KEY` | GitHub App 개인키 PEM 전문 |
| `SANDRONE_WEBHOOK_SECRET` | 웹훅 시크릿 |
| `SANDRONE_MYSQL_DSN`, `SANDRONE_REDIS_ADDR`, `SANDRONE_REDIS_PASSWORD` | 저장소 연결 정보 |
| `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_PASSWORD` | 배포 대상 SSH 접속 정보 |
| `DASHBOARD_USERNAME`, `DASHBOARD_PASSWORD`, `DASHBOARD_SESSION_SECRET` | 관리 콘솔 로그인 |
| `GEMINI_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `NVIDIA_API_KEY` | 최소 한 개 필요 |
| `GHCR_CLEANUP_TOKEN` | `delete:packages` 권한 PAT — 배포 후 패키지 제거용 |

컨테이너 이름·포트·base path·도커 네트워크는 워크플로 상단 `env` 블록에, 모델·엔드포인트·프로바이더 순서는 `internal/adapter/outbound/llm/provider/catalog.go`에 있습니다. 환경변수로는 API 키만 받습니다.

### LLM 기본 모델

기본 모델에는 각 서비스와 모델이 지원하는 요청 옵션을 자동으로 적용합니다.

| 프로바이더 | 기본 모델 | 적용 옵션 |
|---|---|---|
| Gemini | `gemini-3.7-flash` | 샘플링 옵션 생략, `reasoning_effort=medium` |
| Groq | `openai/gpt-oss-120b` | `temperature=1`, `top_p=1`, `reasoning_effort=medium`, 병렬 도구 호출 끄기, `max_completion_tokens` 사용 |
| OpenRouter | `z-ai/glm-5.2:free` | `temperature=1`, `top_p=0.95`, `reasoning.effort=high`, 응답에서 추론 내용 제외 |
| NVIDIA | `google/gemma-4-31b-it` | `temperature=1`, `top_p=0.95`, 사고 내용 출력 끄기, 최대 출력 32,768 토큰 |

환경 변수로 다른 모델을 지정하면 알려지지 않은 모델에는 저장소의 `temperature` 설정만 적용하고, 나머지는 해당 API의 기본 동작을 사용합니다. NVIDIA Gemma 4 호스팅 엔드포인트에는 도구 호출과 JSON 응답 형식 옵션을 보내지 않고 프롬프트 형식으로 처리합니다.

### 리버스 프록시

서버 nginx는 `kimtaeeun.site`의 `/sandrone/`을 컨테이너(`127.0.0.1:10105`)로 그대로 넘깁니다. 앱이 `SANDRONE_BASE_PATH` 아래에서 서비스하므로 프록시는 경로를 자르지 않습니다.

| 경로 | 용도 |
|---|---|
| `https://kimtaeeun.site/sandrone/webhook` | GitHub App 웹훅 URL |
| `https://kimtaeeun.site/sandrone/dashboard` | 관리 콘솔 |
| `https://kimtaeeun.site/sandrone/healthz` | 상태 확인 |

## 저장소별 설정

대상 저장소의 `.reviewbot/config.yml`을 읽습니다. 같은 조직의 `Code-Review-Bot`과 같은 파일을 공유하며, `model`·`baseUrl`·`triggerPrefix`·최상위 `autoReview`는 무시하고 `sandrone:` 블록의 값을 따릅니다.

```yaml
language: ko
minSeverity: minor
maxInlineComments: 25

sandrone:
  autoReview: false        # 자동 리뷰 끄기 (기본값 true)
  autoReviewOnPush: true   # 푸시마다 증분 재리뷰 (기본값 false)
  summaryPlacement: new-comment
  maxReviewBatches: 4        # 큰 PR을 몇 번까지 나눠 검토할지 (기본값 4)
  providers: [gemini, groq]
```

## 라이선스

[MIT](LICENSE)

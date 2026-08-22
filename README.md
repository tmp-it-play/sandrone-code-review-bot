# sandrone-code-review-bot

GitHub Pull Request를 자동으로 리뷰하는 봇입니다. Go로 작성했고 컨테이너 하나로 동작합니다.

## 기능

- PR이 열리면 요약과 인라인 코멘트를 자동으로 남깁니다 (`sandrone.autoReview: true`인 저장소에서)
- `/pr-review`, `/pr-summary`, 리뷰 스레드에서 `/pr-review-reply` — `@sandrone-review-bot` 멘션으로도 호출합니다
- 여러 무료 LLM 프로바이더를 우선순위대로 시도하고, 한도에 걸리면 다음으로 넘어갑니다
- 인라인 코멘트를 달지 못한 지적은 요약 아래 토글로 모아 보여줍니다

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

모델과 포트는 레포지토리 variables(`GEMINI_MODEL`, `DEPLOY_HOST_PORT`, `SANDRONE_BASE_PATH` 등)로 바꿉니다.

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
  autoReview: true
  autoReviewOnPush: true
  summaryPlacement: new-comment
  providers: [gemini, groq]
```

## 라이선스

[MIT](LICENSE)

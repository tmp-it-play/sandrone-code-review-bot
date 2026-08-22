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

# Sandrone Code Review Bot 개인정보처리방침

시행일: 2026년 9월 1일

Sandrone Code Review Bot(이하 "앱")은 `it-play`가 개발하고 운영하는 GitHub App입니다. 이 방침은 앱이 GitHub 저장소와 Pull Request를 처리할 때 어떤 정보를 사용하고 보관하는지 설명합니다.

## 처리하는 정보

앱은 설치 및 사용 과정에서 다음 정보를 처리할 수 있습니다.

- GitHub App 설치 ID, 설치 계정과 저장소 이름, 저장소 공개 여부, 앱 접근 범위
- Pull Request 제목, 본문, 브랜치와 커밋 식별자, 변경 diff, 선택된 저장소 파일, 저장소의 리뷰 설정 및 지침 파일
- Pull Request와 리뷰 스레드의 코멘트, 명령 내용, 작성자와 호출자 GitHub 로그인
- 웹훅 payload와 delivery 식별자, 작업 상태, 리뷰 결과와 finding, 사용한 모델과 token 사용량, 오류 진단 정보

앱은 결제 정보, GitHub 비밀번호 또는 사용자의 GitHub 인증 자격 증명을 직접 수집하지 않습니다.

## 이용 목적

정보는 다음 목적으로만 사용합니다.

- Pull Request 변경 요약, 코드 리뷰, 인라인 finding 및 리뷰 스레드 답글 생성
- 저장소별 설정과 지침 적용
- 중복 요청 방지, 작업 재시도, 게시 결과 복구 및 서비스 보안 유지
- 장애 조사, 사용량 측정 및 서비스 품질 개선

## 외부 처리자

앱은 리뷰 결과를 생성하기 위해 필요한 범위의 Pull Request 및 저장소 내용을 설정된 AI 모델 제공자에게 전송할 수 있습니다. 현재 지원하는 외부 제공자는 Google Gemini, NVIDIA API, OpenRouter, Groq, Mistral AI 및 Cloudflare Workers AI이며, 운영 환경의 로컬 모델이 사용될 수도 있습니다. 실제 요청에는 해당 작업에 선택된 제공자만 사용됩니다.

비공개 저장소 내용은 운영 설정에서 비공개 코드 처리가 명시적으로 허용된 제공자에게만 전송됩니다. 외부 제공자가 처리하는 정보에는 각 제공자의 약관과 개인정보처리방침도 적용됩니다.

앱 운영자는 개인정보를 판매하거나 맞춤 광고에 사용하지 않습니다.

## 보관 기간

- 원본 웹훅 payload는 처리 또는 재시도에 필요한 동안 보관합니다. 정상적인 처리 흐름에서는 수신 후 7일을 처리 기한으로 삼고, 처리 완료 또는 거부 시 payload를 즉시 비웁니다. 서비스 중단이나 backlog로 7일이 지난 대기 항목은 처리가 재개되어 해당 항목을 다시 가져올 때 거부하고 payload를 비우므로, 7일은 강제 삭제 기한이 아닙니다.
- 완료된 웹훅 delivery의 식별자, payload hash와 상태 같은 메타데이터는 완료 시점부터 기본 96시간 동안 보관합니다. 처리할 수 없어 격리된 delivery의 메타데이터와 마스킹된 오류 정보는 거부 시점부터 최대 30일 동안 보관할 수 있습니다. 정상적으로 7일간 재시도한 뒤 거부된 경우 수신 시점부터는 약 37일이 될 수 있습니다.
- 대기, 예약 또는 재시도 중인 queue 작업은 처리가 성공하거나 재시도를 모두 소진할 때까지 전체 작업 payload를 보관합니다. 활성 queue 작업에는 강제 만료 기한이 없으므로 서비스 중단이나 backlog 중에는 처리가 재개될 때까지 장기간 남을 수 있습니다. 성공한 작업 정보는 최대 30일 동안 보관할 수 있습니다. 재시도를 모두 소진한 실패 작업은 전체 작업 payload와 함께 queue archive로 이동하며, 이후 archive 작업 시 약 90일 기준과 10,000개 한도로 정리되거나 운영자가 수동으로 삭제합니다. 이 정리는 개별 작업의 확정 만료 기한이 아니므로 후속 archive 작업이 없으면 더 오래 남을 수 있습니다.
- 리뷰 실행 기록, 생성 결과, finding 및 모델 사용량 기록은 120일에서 180일 사이의 운영 보존 기간을 적용하며, 현재 기본값은 180일입니다.
- 명령 호출 기록, 설치 ID, 계정과 저장소 식별 정보에는 현재 자동 만료가 적용되지 않습니다. 운영자가 수동으로 삭제하거나 확인된 삭제 요청을 처리할 때까지 보관될 수 있습니다.
- 앱이 GitHub에 게시한 코멘트와 리뷰는 GitHub 저장소에 남으며 GitHub의 보관 정책과 저장소 관리자의 조치에 따릅니다.

자동 만료가 설정된 정보는 보존 기간이 끝나면 운영 데이터베이스와 queue에서 정기적으로 삭제합니다. 자동 만료가 없는 정보의 삭제는 아래 연락처로 요청할 수 있습니다. 법적 의무, 보안 조사 또는 분쟁 처리를 위해 필요한 경우에는 해당 목적에 필요한 범위에서 더 오래 보관할 수 있습니다.

## 보안

앱은 GitHub 웹훅 서명을 검증하고, 설치별 단기 토큰과 필요한 최소 GitHub 권한을 사용합니다. 알려진 자격 증명 형식은 모델 입력과 영속적인 리뷰 결과를 만들기 전에 마스킹합니다. 자동 마스킹이 모든 비밀 정보를 탐지한다고 보장할 수 없으므로 Pull Request, 코멘트 또는 저장소 파일에 비밀 정보를 커밋하지 마십시오.

## 선택권과 요청

설치 관리자는 언제든지 GitHub에서 앱의 저장소 접근 범위를 줄이거나 앱을 제거할 수 있습니다. 본인과 관련된 운영 데이터의 확인 또는 삭제, 개인정보 처리에 관한 질문은 아래 연락처로 요청할 수 있습니다. 요청자의 권한을 확인하기 위해 GitHub 계정 또는 설치 정보를 요청할 수 있습니다.

## 방침 변경

처리하는 정보나 운영 방식이 달라지면 이 문서를 갱신하고 시행일을 변경합니다. 중요한 변경은 저장소 또는 앱이 제공하는 적절한 채널을 통해 알립니다.

## 운영자 및 연락처

- 운영자: `it-play`
- 프로젝트: Sandrone Code Review Bot
- 개인정보 문의: [snowykte0426@naver.com](mailto:snowykte0426@naver.com)

---

# Sandrone Code Review Bot Privacy Policy

Effective date: September 1, 2026

Sandrone Code Review Bot (the "App") is developed and operated by `it-play`. This policy explains how the App processes information when it reviews GitHub pull requests.

## Information we process

The App may process:

- GitHub App installation identifiers, account and repository names, repository visibility, and granted access scope
- Pull request titles, bodies, branches, commit identifiers, diffs, selected repository files, and repository review configuration or instruction files
- Pull request and review-thread comments, commands, and GitHub logins of authors and requesters
- Webhook payloads and delivery identifiers, job state, review results and findings, model and token usage, and diagnostic error information

The App does not directly collect payment information, GitHub passwords, or a user's GitHub authentication credentials.

## How we use information

We use this information only to:

- Generate pull request summaries, code reviews, inline findings, and review-thread replies
- Apply repository-specific configuration and instructions
- Prevent duplicate work, retry jobs, recover publication state, and protect the service
- Investigate failures, measure usage, and improve service quality

## Service providers

The App may send the minimum pull request and repository content needed to generate a result to configured AI model providers. Supported external providers currently include Google Gemini, NVIDIA API, OpenRouter, Groq, Mistral AI, and Cloudflare Workers AI. A locally operated model may also be used. Only providers selected for the particular request are used.

Private-repository content is sent only to providers that the operator has explicitly allowed to process private code. Each external provider's own terms and privacy policy also apply to information it processes.

The operator does not sell personal information or use it for targeted advertising.

## Retention

- Raw webhook payloads are kept while needed for processing or retries. In normal operation, seven days after receipt is the processing deadline, and the payload is cleared immediately when processing completes or the delivery is rejected. If an outage or backlog leaves a pending delivery past seven days, the worker rejects it and clears the payload when processing resumes and the delivery is claimed; seven days is therefore not a hard deletion deadline.
- Metadata for a completed webhook delivery, such as its identifier, payload hash, and status, is normally retained for 96 hours after completion. Metadata and masked error information for a rejected delivery may be retained for up to 30 days after rejection. A delivery rejected after the normal seven-day processing window may therefore be retained for about 37 days from receipt.
- A pending, scheduled, or retrying queue task retains its full job payload until processing succeeds or all retries are exhausted. Active queue tasks have no hard expiration, so they may remain for an extended period during an outage or backlog until processing resumes. Successful task information may be retained for up to 30 days. A task that exhausts all retries is moved to the queue archive with its full job payload. Archived tasks are trimmed during later archive operations using an approximately 90-day cutoff and a 10,000-item limit, or deleted manually by the operator. This is not a guaranteed per-task expiration, so an archived task may remain longer when no later archive operation triggers trimming.
- Review runs, generated results, findings, and model-usage records use an operational retention period between 120 and 180 days; the current default is 180 days.
- Command-invocation records, installation identifiers, and account or repository identifiers currently have no automatic expiration. They may be retained until the operator deletes them manually or completes a verified deletion request.
- Comments and reviews published to GitHub remain in the GitHub repository and are governed by GitHub's retention practices and repository administrators.

Information with a configured expiration is periodically removed from the operational database and queue. You can request deletion of information without an automatic expiration by using the contact below. Information may be kept longer when necessary for a legal obligation, security investigation, or dispute.

## Security

The App validates GitHub webhook signatures, uses installation-scoped short-lived tokens, and requests only the GitHub permissions needed for its features. Known credential patterns are masked before model prompts and persistent review results are created. Automated masking cannot detect every secret, so secrets should not be committed to pull requests, comments, or repository files.

## Your choices and requests

Installation administrators can reduce repository access or uninstall the App at any time through GitHub. To request access to or deletion of operational data related to you, or to ask a privacy question, use the contact below. We may ask for GitHub account or installation information to verify authority over the request.

## Changes to this policy

We will update this document and its effective date when our processing or operating practices change. Material changes will be announced through an appropriate repository or App channel.

## Operator and contact

- Operator: `it-play`
- Project: Sandrone Code Review Bot
- Privacy contact: [snowykte0426@naver.com](mailto:snowykte0426@naver.com)

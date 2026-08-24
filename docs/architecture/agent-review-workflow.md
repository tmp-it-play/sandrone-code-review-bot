# 모든 PR의 bounded 에이전트 리뷰 워크플로

- 설계 ID: REVIEW-WORKFLOW-001
- 상태: 승인된 운영 구조
- 구현 상태: main 반영, 운영 schema migration과 실제 배포 health 검증 완료
- 최초 작성일: 2026-08-24
- 최종 갱신일: 2026-08-24
- 결정 소유자: Sandrone maintainers
- 적용 범위: 크기와 유형에 관계없는 모든 Pull Request 자동·수동 코드 리뷰

## 1. 목적과 확정 결정

Sandrone은 모든 PR을 코드가 통제하는 하나의 bounded agentic workflow로 처리한다. 작은 PR과 대형 PR에 별도 파이프라인을 두지 않는다. 작은 PR은 같은 상태 머신에서 review unit과 모델 호출 수가 자연스럽게 하나로 줄어든다.

확정 결정은 다음과 같다.

- MySQL이 run, unit, coverage, 검증, finding lifecycle과 publication checkpoint의 source of truth다.
- 에이전트는 고정된 PR snapshot을 읽고 구조화된 후보를 만들 뿐 queue, database와 GitHub를 직접 변경하지 않는다.
- 코드가 계획, lease, retry, coverage 판정, 검증 gate, 중복 억제, 게시와 cleanup을 소유한다.
- 성공한 unit은 같은 run의 retry에서 다시 호출하지 않는다.
- 다른 run의 AI 결과를 재사용하지 않아 hard TTL과 현재 코드 근거를 우회하지 않는다.
- 부분 검토는 complete로 표시하지 않고 complete watermark를 전진시키지 않는다.
- 공개 finding은 필수 필드와 exact added-line evidence를 만족해야 하며, 잘못 기입된 위치는 같은 파일에서 근거가 유일하게 일치할 때만 실제 추가 줄로 재앵커한다.
- 조건을 만족하는 독립 provider가 있으면 verifier를 실행하고 supported 판정만 게시한다.
- 분석량과 게시량 모두 durable hard cap으로 제한한다.
- 구조화된 리뷰 데이터는 기본 180일, 설정 가능한 120~180일의 sliding이 아닌 hard TTL을 가진다.

주요 구현:

- [ReviewPullRequest use case](../../internal/core/usecase/reviewpullrequest/usecase.go)
- [Review workflow persistence](../../internal/adapter/outbound/persistence/mysql/reviewworkflowrepository.go)
- [Workflow domain](../../internal/core/reviewworkflow/runstatus.go)

## 2. 문제 사례

[gikipedia-server PR #13](https://github.com/wntopia/gikipedia-server/pull/13)은 변경량이 많을 때 기존 직렬 리뷰 구조의 누락이 크게 드러난 사례다. 릴리스 PR이라는 성격은 본질이 아니다.

- 전체 변경은 159개 파일, +5,899/-906줄이었다.
- 제거 파일과 patch가 없는 파일을 제외한 리뷰 가능 파일은 127개였다.
- 실제 성공 결과에 포함된 파일은 25개로 리뷰 가능 파일의 약 19.7%였다.
- 87개 파일은 파일 수 상한으로 모델 호출 전에 제외됐다.
- 선택된 파일 중 15개는 모델 호출 실패가 발생한 배치와 함께 제외됐다.
- 미검토 파일의 변경량은 +3,336/-42줄이었다.
- 최종 인라인 지적은 2개였고 리뷰 본문의 대부분은 102개 미검토 파일 목록이었다.

[게시된 리뷰](https://github.com/wntopia/gikipedia-server/pull/13#pullrequestreview-5002638113)는 일부 핵심 경로를 검토하지 못했지만 전체 변경을 확정적으로 설명했다. 두 인라인 지적도 검증 경계의 필요성을 보여준다.

- [전체 데이터를 메모리에 적재하는 정합성 배치](https://github.com/wntopia/gikipedia-server/pull/13#discussion_r3838779100)는 diff 근거가 있는 문제였다.
- [JVM target 불일치 지적](https://github.com/wntopia/gikipedia-server/pull/13#discussion_r3838779103)은 build JVM과 bytecode target의 차이를 오류로 단정한 false positive였다.

따라서 해결 대상은 context 크기 하나가 아니라 작업 분할, 성공 상태 보존, 근거 검증, partial 의미론, 중복 제거, 게시 멱등성과 보존 정책 전체다.

## 3. 단일 운영 흐름

모든 리뷰 요청은 다음 흐름을 따른다.

Webhook inbox → immutable ReviewRun → diff와 coverage 수집 → deterministic risk/path planning → 최대 8개 ReviewUnit → exact evidence gate → open occurrence dedupe → independent verifier → deterministic root-cause reduce → 최대 25개 finding → canonical publication checkpoint → GitHub review 또는 fallback comment → terminal state와 cleanup

### 3.1 Intake와 snapshot

- 서명을 검증한 webhook은 payload와 identity를 MySQL에 먼저 저장한 뒤 202를 반환한다.
- dispatcher가 token-fenced lease로 inbox를 처리하므로 Redis enqueue 장애가 webhook 수신 완료를 잃게 만들지 않는다.
- run은 base SHA, head SHA, config hash, prompt version, model policy hash와 request identity로 구분한다.
- 모든 diff, source read, tool read와 publication position은 run의 고정 base/head snapshot을 사용한다.
- 저장소 설정과 instruction은 변경 head보다 base SHA와 base ref를 먼저 읽어 PR 코드가 자기 리뷰 정책을 주입하지 못하게 한다.
- 모델 호출 전후와 게시 직전에 live base/head 및 latest-run fence를 다시 확인한다.
- 더 최신 snapshot이 확인되면 stale run을 superseded 처리하고 게시하지 않는다.

관련 구현:

- [Webhook inbox model](../../internal/adapter/outbound/persistence/mysql/model/webhookdelivery.go)
- [Webhook inbox repository](../../internal/adapter/outbound/persistence/mysql/webhookinboxrepository.go)
- [Policy ref selection](../../internal/core/pullrequest/target.go)
- [Repository config loader](../../internal/adapter/outbound/settings/configloader.go)

### 3.2 Coverage와 deterministic planning

- PR files와 compare 결과를 수집하고 불완전한 compare는 전체 PR file 목록으로 보수적으로 보완한다.
- 각 hunk, patch gap과 patch가 없는 file change에 deterministic coverage key를 부여한다.
- include/exclude, max files, patch truncation, manifest gap, prompt limit과 unit 실패를 coverage reason으로 남긴다.
- 파일은 보안, 인증, 데이터, 동시성, API, 배포 위험 신호를 우선하고 같은 directory의 구현과 test를 인접하게 정렬한다.
- 실제 message 크기는 repository config와 첫 eligible provider의 prompt cap 안에서 계산한다.
- 다른 provider로 fallback할 때는 그 provider의 request profile에 맞지 않는 큰 unit을 호출하지 않고 건너뛴다.
- 필수 diff가 한 provider prompt에 들어가지 않으면 조용히 자르지 않고 prompt_limit deferred로 남긴다.
- unit hash는 배정된 coverage key 집합으로 결정한다.

이 planning은 LLM 호출이 아니라 결정론적 코드다.

관련 구현:

- [Semantic file planner](../../internal/core/usecase/reviewpullrequest/semanticfileplanner.go)
- [Batch planner](../../internal/core/usecase/reviewpullrequest/reviewbatchplanner.go)
- [Coverage plan builder](../../internal/core/reviewworkflow/planbuilder.go)
- [Unit input hash](../../internal/core/usecase/reviewpullrequest/reviewunithash.go)

### 3.3 Unit review와 checkpoint

- 하나의 review job 안에서 unit을 순서대로 처리한다.
- 각 unit은 lease, attempt count, input hash, provider/model usage와 구조화된 ResultJSON을 가진다.
- input hash가 같은 succeeded unit은 process restart와 queue retry에서도 다시 호출하지 않는다.
- checkpoint JSON이 손상됐거나 input hash가 달라지면 해당 unit만 다시 실행한다.
- 실패 unit은 coverage를 failed로 전환하고 다른 성공 unit 결과는 유지한다.
- 각 unit은 별도 read_file executor를 받는다.
- PR 전체 변경 파일이 8개 이하일 때만 summary.files 스키마와 파일별 요약 지시를 prompt에 포함한다. 9개 이상이면 해당 필드와 지시 자체를 넣지 않고 현재 unit의 짧은 overview와 finding에 출력 예산을 사용하며, 실제 검토 범위는 model output이 아니라 coverage ledger로 판정한다.
- 여러 unit의 overview는 추가 모델 호출 없이 하나의 연속된 문단으로 합치며 내부 unit 번호나 배치 구조를 공개 본문에 노출하지 않는다.
- reviewer response는 JSON 문법, 필수 overview와 Finding 필드 semantic validation을 통과해야 한다. 일부 malformed finding은 버리고 유효한 finding을 유지한다. 게시 severity 기준을 충족한 후보가 있었지만 하나도 exact evidence로 고정하거나 안전하게 재앵커할 수 없으면 `all_findings_unanchored` semantic failure로 처리해 같은 호출 상한 안에서 다음 eligible provider로 넘긴다.

Cross-run candidate/result cache는 저장하거나 읽지 않는다.

관련 구현:

- [Unit checkpoint flow](../../internal/core/usecase/reviewpullrequest/usecase.go)
- [Unit result model](../../internal/adapter/outbound/persistence/mysql/model/reviewunit.go)
- [LLM chain](../../internal/adapter/outbound/llm/chain/chain.go)

### 3.4 Finding 검증, 중복 제거와 reduce

Finding은 다음 순서로 처리한다.

1. file, line, title, body, evidence, rootCause와 severity의 필수성 검증
2. finding evidence가 지정 위치의 실제 추가 줄과 byte-for-byte 일치하는지 검증
3. 지정 위치가 틀렸더라도 같은 파일의 추가 줄에서 동일 evidence가 유일하게 발견될 때만 실제 위치로 재앵커
4. repository minimum severity 적용
5. 현재 open exact occurrence와 같은지 중복 검증
6. severity 우선, root-cause round-robin으로 verifier 입력을 최대 25건으로 제한
7. reviewer에 참여하지 않은 eligible provider가 있으면 독립 verifier 실행
8. supported finding만 root-cause fingerprint로 결정론적으로 병합
9. 실제 commentable position만 inline으로 배치

거리나 유사도로 가장 가까운 hunk 또는 줄을 고르지 않는다. 같은 evidence가 여러 위치에 있어 하나로 확정할 수 없거나 exact match를 찾지 못하면 게시하지 않는다. 검증 결과는 exact, normalized, reanchored, unknown_file, invalid_span, non_added_span, not_found와 ambiguous로 나누어 운영 로그에 기록한다.

독립 verifier의 의미론은 fail-safe다.

- 독립 provider가 없으면 이미 통과한 deterministic exact-evidence 결과를 사용한다.
- 독립 provider가 있으면 supported만 게시한다.
- 호출 예산 부족, prompt overflow, provider 오류, incomplete 응답 또는 parse 오류가 발생하면 verifier 대상 finding을 모두 보류하고 run을 partial로 만든다.
- verifier 완료 결과와 실패 시도 usage는 review_verifications에 checkpoint한다.

Root-cause reduce는 추가 LLM 호출을 만들지 않는다. 같은 root cause의 여러 occurrence를 한 finding으로 모으고 severity, suggestion 유무, 근거 길이와 위치에 따른 결정론적 대표를 고른다.

관련 구현:

- [Finding validity](../../internal/core/review/finding.go)
- [Exact evidence verifier](../../internal/core/reviewanalysis/evidenceverifier.go)
- [Independent verifier](../../internal/core/usecase/reviewpullrequest/findingverifier.go)
- [Root-cause reducer](../../internal/core/dedupe/rootcausereducer.go)
- [Exact position mapper](../../internal/core/mapping/positionmapper.go)
- [Finding bounds](../../internal/core/usecase/reviewpullrequest/reviewresultbounds.go)

### 3.5 Terminal 상태와 coverage 의미

ReviewRun 상태:

- planning
- running
- publishing
- complete
- partial
- failed
- superseded
- cancelled
- skipped

ReviewUnit 상태:

- pending
- running
- succeeded
- failed
- deferred

Coverage 상태:

- indexed
- planned
- reviewed
- deep_reviewed
- failed
- deferred
- skipped
- superseded

Complete는 모든 eligible coverage가 reviewed 또는 deep_reviewed일 때만 가능하다. 일부만 성공하면 partial, 성공한 coverage가 없으면 failed다. 분석 대상 denominator가 0이고 모든 제외 사유가 명시된 경우만 skipped다. Complete와 정상 zero-denominator skipped만 complete watermark를 전진시킨다.

Coverage ledger와 total, reviewed, failed, deferred, skipped 같은 내부 카운터는 상태 판정과 retry에만 사용하고 공개 Review 본문에는 표시하지 않는다. 실제로 다루지 못한 파일이 있으면 bounded reason별 집계와 파일 예시만 접은 영역으로 표시한다.

Complete는 eligible coverage의 분석 완료를 뜻하며 모든 candidate finding의 공개를 뜻하지 않는다. 분석을 끝낸 뒤 verifier와 publication의 25건 예산에서 제외된 하위 후보 수는 omitted로 표시하지만 coverage를 partial로 바꾸거나 complete watermark를 막지 않는다. 분석 coverage와 공개 코멘트 예산을 결합해 같은 코드를 반복 호출하지 않는다.

max_files, max_batches 또는 prompt_limit로 deferred된 범위는 같은 head의 재시도에서 자동 회전하지 않는다. 반복 호출로 run당 비용 상한을 우회하지 않기 위한 확정 경계이며 해당 run은 partial로 남고 complete watermark를 전진시키지 않는다. 이 상한을 넘는 변경은 PR을 나누어야 한다.

## 4. Hard limit

| 대상 | 기본값 또는 상한 | 적용 의미 |
| --- | ---: | --- |
| Worker concurrency | 2 | CD와 application 기본값 |
| 선택 파일 | 최대 160 | 초과 파일은 max_files deferred |
| Review unit | 1~8, 기본·상한 8 | 초과 unit은 max_batches deferred |
| Repository 요청 output | 최대 8,192 tokens | provider profile이 더 작으면 provider 한도 우선 |
| 파일별 변경 요약 | PR 전체 변경 파일 최대 8개 | 9개 이상이면 prompt에 요청하지 않고 coverage ledger 사용 |
| Unit read_file | 최대 6회 | unit별 executor에 적용 |
| Tool round | 최대 2회 | chain 내부 왕복 상한 |
| Unit tool output | 합계 2,000 bytes | alias 복원과 masking 이후 누적 상한 |
| Review 외부 호출 | run당 최대 12회 | model, retry, fallback, verifier와 tool execution을 모두 합산 |
| Verifier 입력 finding | 최대 25개 | severity와 root-cause 분산 순서 |
| 최종 publication finding | 최대 25개 | inline과 fallback 합계의 상한 |
| Inline comment | 최대 25개 | repository 설정으로 더 낮출 수 있음 |
| Occurrence 재검증 | 최대 500건 | 변경 경로에 한정 |
| Occurrence source read | 최대 32개 경로, 파일당 1 MiB | 불확실하면 open 유지 |
| Review와 provider usage retention | 기본 180일 | 120~180일로 clamp |
| Cleanup | 24시간마다, review 10건·provider usage 500건 batch | 작은 transaction으로 반복 |

External call budget은 review_runs.external_calls에서 MySQL 조건부 update로 예약한다. 같은 run의 queue retry도 같은 counter를 사용한다. Summary와 reply도 각 publication checkpoint의 durable counter로 operation당 같은 12회 상한을 사용한다.

관련 구현:

- [Application config](../../internal/bootstrap/config.go)
- [Repository defaults](../../internal/core/setting/repoconfig.go)
- [Durable review call budget](../../internal/adapter/outbound/persistence/mysql/reviewcallbudgetrepository.go)
- [Path-mapping tool executor](../../internal/core/usecase/reviewpullrequest/pathmappingtoolexecutor.go)
- [Retention worker](../../internal/adapter/inbound/maintenance/reviewretentionworker.go)

## 5. Provider와 routing

다음 provider는 모두 정식 catalog 구성이다. Application startup과 CD는 아래 credential 전체를 필수로 검증하며, 로드된 provider는 역할 및 privacy 정책에 따라 route에 참여한다. 별도 활성화 allowlist는 없다.

표의 prompt와 output 값은 외부 서비스의 요금제 보장이 아니라 현재 application catalog와 request profile에 고정한 상한이다.

| 순서 | Provider | Model | 허용 역할 | Prompt chars | Provider output profile |
| ---: | --- | --- | --- | ---: | ---: |
| 1 | Gemini | gemini-3.7-flash | planner, reviewer, verifier, reducer, summary, reply | 600,000 | 65,536 |
| 2 | NVIDIA | google/gemma-4-31b-it | reviewer, summary, reply | 180,000 | 32,768 |
| 3 | OpenRouter | z-ai/glm-5.2:free | planner, reviewer, verifier, reducer, summary, reply | 180,000 | 131,072 |
| 4 | Groq | openai/gpt-oss-120b | planner, reviewer, verifier, reducer, summary, reply | 12,000 | 2,500 |
| 5 | Mistral | mistral-small-2603, Mistral Small 4 | planner, reviewer, verifier, reducer, summary, reply | 600,000 | 8,192 |
| 6 | Cloudflare GLM | @cf/zai-org/glm-4.7-flash | reviewer, summary, reply | 450,000 | 8,192 |
| 7 | Cloudflare Gemma | @cf/google/gemma-4-26b-a4b-it | reviewer, summary, reply | 450,000 | 8,192 |

Repository의 MaxOutputTokens는 최대 8,192이므로 실제 요청은 repository 한도와 provider output profile 중 더 작은 값에 맞는다.

허용 역할은 catalog capability를 뜻한다. 현재 review workflow는 LLM planner와 LLM reducer를 호출하지 않는다.

Credential:

- GEMINI_API_KEY
- NVIDIA_API_KEY
- OPENROUTER_API_KEY
- GROQ_API_KEY
- MISTRAL_API_KEY
- CLOUDFLARE_API_TOKEN
- CLOUDFLARE_ACCOUNT_ID

CLOUDFLARE_API_TOKEN은 CLOUDFLARE_ACCOUNT_ID와 같은 account 범위의 [Workers AI 권한](https://developers.cloudflare.com/ai-gateway/usage/rest-api/)을 가져야 한다. AI Gateway 권한만 있는 token은 direct `/accounts/{account_id}/ai/*` endpoint를 인증할 수 없다.

공개 코드는 역할 capability를 만족하는 활성 provider로 route할 수 있다. 비공개 코드는 운영자가 데이터 처리 조건을 확인해 SANDRONE_PRIVATE_CODE_PROVIDERS에 명시한 provider에만 전송한다. Allowlist가 비어 있거나 해당 역할을 수행할 provider가 없으면 fail closed한다.

[Mistral Free mode](https://help.mistral.ai/en/articles/455207-can-i-opt-out-of-my-input-or-output-data-being-used-for-training)는 기본 privacy 설정에서 API input과 output을 모델 개선에 사용할 수 있다. 비공개 코드 allowlist에 mistral을 넣기 전에 Mistral Admin의 Privacy 설정에서 학습 사용을 opt out해야 한다. Cloudflare는 현재 AI Gateway를 거치지 않는 [Workers AI direct endpoint](https://developers.cloudflare.com/workers-ai/platform/data-usage/)를 사용한다.

Batch 계획은 첫 eligible provider의 cap과 repository config를 사용한다. Fallback provider마다 완성된 message와 tool definition 크기를 다시 검사하므로 Groq처럼 작은 cap의 provider는 큰 batch에서 호출 없이 건너뛴다. 전체 provider 가운데 가장 작은 cap으로 모든 batch를 축소하지 않는다.

ForceJSON 요청에서 Gemini 3.7은 공식 [function calling과 structured output 조합](https://ai.google.dev/gemini-api/docs/function-calling#function_calling_with_structured_output)을 사용하고 Mistral도 tool definition이 함께 있는 JSON mode를 유지한다. Gemini 3 tool call의 [thought signature](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures#signatures-for-openai-compatibility)는 다음 assistant turn에 손실 없이 돌려보낸다. Mistral tool result가 빈 문자열이어도 필수 content 필드를 생략하지 않는다. NVIDIA Gemma 4 31B는 tool calling을 끄고 provider의 [structured generation](https://docs.nvidia.com/nim/large-language-models/1.15.0/structured-generation.html)을 사용해 순수 JSON을 강제한다. 비 JSON 응답에는 [문서화된 빈 thought channel](https://docs.api.nvidia.com/nim/reference/google-gemma-4-31b-it)이 선두에 붙을 수 있어 해당 exact prefix만 client boundary에서 제거하며, 임의 prose나 Markdown fence에서 JSON을 추출하지 않는다. Provider의 Retry-After는 transient retry 대신 fallback에 반영하고 설정된 cooldown보다 길 때 최대 24시간까지 유지한다. 새 실패의 cooldown이 기존 deadline보다 짧으면 기존 값을 줄이지 않는다. 같은 credential과 quota를 쓰는 Cloudflare GLM과 Gemma는 하나의 failure domain으로 취급해 auth, quota 또는 rate-limit 실패 시 둘 다 cooldown한다.

관련 구현:

- [Provider catalog](../../internal/adapter/outbound/llm/provider/catalog.go)
- [Provider request profiles](../../internal/adapter/outbound/llm/provider/requestprofilecatalog.go)
- [Role and privacy routing](../../internal/adapter/outbound/llm/chain/chain.go)

## 6. 보안과 tool 격리

- PR 제목, 본문, diff, 파일 내용, instruction과 모델 output을 신뢰할 수 없는 입력으로 취급한다.
- Prompt message 전체를 provider 호출과 input hash 계산 전에 masking한다.
- Tool result는 head SHA에 고정된 read_file만 허용하고 alias 복원, secret masking과 누적 2,000-byte 제한을 적용한다.
- 모델 output은 parse, checkpoint와 publication 전에 다시 masking한다.
- Secret이 포함된 path는 안정적인 private alias로 바꾸고 model result와 tool argument에서만 원래 path로 복원한다.
- Multiline private key masking은 CR, LF, CRLF와 diff의 선두 +, -, space를 보존하면서 각 줄 내용을 REDACTED로 바꾼다.
- Agent에 shell, GitHub write, database, provider credential과 임의 network access를 제공하지 않는다.
- GitHub write는 deterministic publisher adapter만 수행한다.

관련 구현:

- [Secret masker](../../internal/adapter/outbound/masking/secretmasker.go)
- [Prompt path aliases](../../internal/core/usecase/reviewpullrequest/promptpathmap.go)
- [GitHub read_file executor](../../internal/adapter/outbound/forge/github/readfileexecutor.go)

## 7. Finding identity와 lifecycle

Root-cause fingerprint와 occurrence fingerprint를 분리한다.

- Root-cause fingerprint는 rootCause의 대소문자를 정규화하고 whitespace를 접되 punctuation을 보존한다.
- Occurrence fingerprint는 root-cause fingerprint, case-sensitive path, line, end line과 exact evidence를 포함한다.
- 중복 억제에는 유효 기간 안의 open occurrence의 current fingerprint만 사용한다.
- provisional, failed와 superseded review/run의 occurrence는 중복 억제에 포함하지 않는다.

새 run에서는 변경되거나 제거된 경로의 기존 open occurrence만 bounded 재검증한다.

- 현재 head의 masked file content에서 evidence를 줄바꿈만 LF로 정규화한 뒤 연속된 줄로 exact match한다.
- 공백은 정규화하지 않는다.
- 앞줄 삽입으로 evidence 위치만 이동하면 같은 root/path/evidence 그룹에서 기존 line에 가장 가까운 미사용 match를 1:1로 배정하고 current fingerprint를 재앵커한다.
- 동일 evidence가 더 많이 생기면 기존 occurrence 수를 넘는 위치는 신규 occurrence로 게시할 수 있다.
- evidence가 읽을 수 있는 변경 파일에서 사라지면 resolved로 전환한다.
- removed path와 rename의 previous path는 resolved로 전환한다.
- file fetch 오류, 1 MiB 초과 또는 read cap 소진으로 내용을 확정하지 못하면 open을 유지한다.
- resolved occurrence의 exact 문제가 다시 도입되면 억제되지 않고 게시 후 reopen된다.

검증되어 실제 publication 대상이 된 finding은 provisional review 저장과 같은 transaction에서 occurrence를 upsert한다. 다만 active query는 source review가 succeeded 또는 partial이고 source run이 complete 또는 partial인 row만 사용하므로 미게시 실행이 기존 open 상태를 오염시키지 않는다.

Occurrence expiry는 source run의 StartedAt + retention으로 최초 저장 시 고정하고 retry로 연장하지 않는다. Source run의 terminal expiry가 더 이르면 그 시각이 읽기 hard cap이다.

관련 구현:

- [Fingerprint](../../internal/core/review/fingerprint.go)
- [Occurrence lifecycle](../../internal/core/usecase/reviewpullrequest/occurrencelifecycle.go)
- [Occurrence repository](../../internal/adapter/outbound/persistence/mysql/findingoccurrencerepository.go)
- [Atomic occurrence writer](../../internal/adapter/outbound/persistence/mysql/findingoccurrencewriter.go)

## 8. Persistence와 checkpoint

MySQL schema에 다음 상태가 영속화된다.

| Table 또는 storage | 역할 |
| --- | --- |
| review_runs | immutable snapshot, terminal status, coverage aggregate, lease, external call counter와 expiry |
| review_units | unit hash/input hash, lease, attempts, provider usage와 ResultJSON |
| coverage_items | hunk, file change와 patch gap별 eligibility, status와 reason |
| review_verifications | verifier input hash, supported occurrence IDs, attempts, usage와 completion checkpoint |
| reviews | run의 terminal projection과 provider attribution |
| findings | 게시 finding, evidence, rootCause와 occurrence JSON |
| finding_occurrences | open/resolved lifecycle, current anchor, source와 immutable expiry |
| review_publications | canonical review body, inline JSON, fallback body, payload hash, marker와 GitHub receipt |
| review_publication_invalidations | stale publication 보상 retry와 expiry |
| pull_request_states | latest run, complete watermark, publication lease와 summary result/publication checkpoint |
| reply_publications | reply operation lease, canonical result, provider usage, receipt와 expiry |
| migration_checkpoints | bounded backfill cursor와 completion |
| webhook_deliveries | durable ingress payload, lease, retry와 terminal tombstone |

Review publication은 외부 write 전에 canonical body, inline comments, fallback body, final status와 payload hash를 review_publications에 저장한다. Marker 조회와 stored payload 재게시로 응답 유실과 process restart를 복구한다. Channel과 GitHub ID가 저장되기 전에는 publishing run을 terminal로 끝내지 않는다.

Summary는 pull_request_states의 summary operation/result fields에 canonical content, input hash, provider usage, lease와 completion을 저장한다. Reply는 reply_publications에 같은 성격의 operation checkpoint를 저장한다. 세 publication 경로 모두 모델 성공 결과를 외부 게시보다 먼저 durable하게 만든다.

관련 구현:

- [Schema migration](../../internal/adapter/outbound/persistence/mysql/connection.go)
- [Review publication repository](../../internal/adapter/outbound/persistence/mysql/reviewpublicationrepository.go)
- [Summary publication repository](../../internal/adapter/outbound/persistence/mysql/summarypublicationrepository.go)
- [Reply publication repository](../../internal/adapter/outbound/persistence/mysql/replypublicationrepository.go)

## 9. Hard TTL, cleanup과 bounded backfill

- Review retention 기본값은 180일이며 application config가 120~180일로 clamp한다.
- Terminal review run expiry는 terminal timestamp + retention으로 고정한다.
- Unit, coverage, verifier, review, finding과 review publication은 run 삭제와 cascade되거나 자기 expiry 중 더 이른 실효 시각에 제거된다.
- Summary와 reply checkpoint도 생성 시 고정한 expiry를 사용하고 조회로 연장하지 않는다.
- Finding occurrence는 source run StartedAt 기준의 immutable expiry를 사용한다.
- Cross-run AI result reuse를 하지 않는다.
- Provider usage도 review와 같은 retention cutoff로 삭제하므로 dashboard 수치는 현재 보존 구간만 집계한다.
- Retention worker는 시작 시와 이후 24시간마다 review 10건, provider usage 500건 batch로 orphan 종료와 expired data 삭제를 반복하며 두 cleanup의 실패를 서로 격리한다.
- 7일 동안 heartbeat와 유효 lease가 없는 nonterminal run은 failed로 종료해 TTL을 시작한다.
- 만료 데이터는 중복 억제, incremental baseline과 lifecycle 판단에 사용하지 않는다.

기존 finding은 HTTP startup을 막는 전수 migration으로 변환하지 않는다.

- Schema AutoMigrate는 MySQL GET_LOCK으로 직렬화한다.
- Finding occurrence backfill은 maintenance worker가 50건씩 처리한다.
- migration_checkpoints의 monotonic cursor로 restart 이후 재개한다.
- Run이 있는 legacy finding은 원 run의 StartedAt과 retention으로 expiry를 복원하고 terminal run expiry보다 길게 만들지 않는다.
- Run이 없는 legacy review는 원 finished timestamp와 120일 기준을 사용한다.
- Stored old fingerprint ID를 신뢰하지 않고 저장된 path, line, evidence와 rootCause로 현재 fingerprint를 다시 계산한다.
- Backfill checkpoint가 완료되기 전에는 cursor 이후의 성공/부분 완료 finding을 최대 5,000건만 읽는 bounded fallback이 exact duplicate suppression을 보완한다.
- Checkpoint CompletedAt이 기록되는 즉시 fallback은 꺼지며 이후 lifecycle의 resolve/reopen 상태를 다시 덮지 않는다.

관련 구현:

- [Occurrence backfill](../../internal/adapter/outbound/persistence/mysql/findingoccurrencebootstrap.go)
- [Backfill checkpoint](../../internal/adapter/outbound/persistence/mysql/findingoccurrencebootstraprepository.go)
- [Bounded migration fallback](../../internal/adapter/outbound/persistence/mysql/findingoccurrencefallback.go)

## 10. Publication과 stale 보상

- Agent는 GitHub에 쓰지 않는다.
- Publisher는 PR별 latest-run과 token-fenced publication lease를 확인한다.
- Review marker와 payload hash로 동일 run의 중복 게시를 막는다.
- GitHub review 전체가 유효하지 않은 inline 때문에 거절되면 stored fallback body를 issue comment로 게시한다.
- 게시 직후 live base/head 또는 desired latest run이 달라졌으면 같은 marker의 bot-owned review body, top-level inline과 issue comment를 무효화한다.
- 일부 무효화가 실패하면 성공으로 숨기지 않고 publication invalidation debt로 기록해 hard TTL 안에서 재시도한다.
- Review 본문과 내부 terminal status가 completeness의 source of truth다.

Summary와 thread reply도 operation marker, lease, canonical result checkpoint와 completed tombstone으로 중복 게시를 방지한다.

## 11. 의도적으로 사용하지 않는 구조

다음 구조는 현재 비용·복잡도 경계상 사용하지 않는다.

- LLM planner: risk/path deterministic planner가 unit을 구성한다.
- Specialist agent: specialist 위험 관점을 reviewer prompt에 포함하고 별도 호출을 만들지 않는다.
- LLM reducer: exact evidence, fingerprint, severity와 root-cause 규칙으로 결정론적으로 reduce한다.
- Distributed unit queue와 fan-out dispatcher: 최대 8개 unit을 하나의 task에서 처리하고 MySQL checkpoint로 재개한다.
- Installation별 weighted fair queue: worker concurrency 2와 provider cooldown으로 bounded 처리한다.
- Source snapshot LRU: 고정 SHA content adapter와 unit별 bounded read를 사용한다.
- GitHub Check: compact review coverage와 내부 terminal status를 사용한다.
- 자동 concurrency 또는 비용 tuner: workers 2, units 8와 calls 12를 고정한다.
- Cross-run AI result cache: hard TTL과 현재 evidence 의미를 지키기 위해 사용하지 않는다.

이 경계는 미완료 기능 목록이 아니라 운영 비용과 장애 표면을 제한하는 설계 결정이다.

## 12. 배포

배포는 중단을 허용하는 bounded stop/remove → start → health 절차다. 구·신 worker를 동시에 운영하거나 이전 container를 자동 복구하지 않는다.

전체 원격 배포는 Ubuntu runner의 2,100초 제한과 job 45분 제한 안에서 실행한다. 배포 서버에는 별도 timeout 도구나 watchdog process를 요구하지 않는다.

1. main push가 gofmt, go test, go vet와 build gate를 통과한다.
2. ARM64 commit-SHA image를 build하고 GHCR에 push한다.
3. 배포 서버가 새 image를 먼저 pull한다.
4. 기존 sandrone container에 SIGTERM 기반 최대 180초 stop을 적용하고 제거한다.
5. 새 container 하나를 같은 port와 network에서 시작한다.
6. container 내부 healthz를 최대 30회 확인한다.
7. Health가 실패하면 로그를 남기고 실패 container를 제거한 뒤 CD를 실패 처리한다.

이 절차에는 서비스 중단 구간이 있다. Health 실패 시 이전 container나 image를 자동 재시작하지 않는다. 사용 중이지 않은 과거 commit-SHA image만 배포 후 정리한다.

Schema migration은 새 process startup에서 GET_LOCK과 bounded AutoMigrate로 실행한다. 같은 물리 connection을 고정하되 lock 조회, migration, backfill, lock 해제는 새 GORM statement session으로 격리한다. Migration과 backfill이 끝나면 과거 실패가 만든 `null_int64` table을 같은 lock 안에서 삭제하고 부재를 검증한다. 대량 finding occurrence 변환은 startup 이후 checkpoint worker가 처리하므로 migration 범위가 startup 시간을 무제한 늘리지 않는다.

현재 상태:

- 로컬과 GitHub Actions의 gofmt, go test, go vet와 build 검증 완료
- main의 stop/remove → start → health CD 완료
- 운영 MySQL 구 스키마 migration과 보존 기준 시각 backfill 완료

관련 구현:

- [CD workflow](../../.github/workflows/cd.yml)
- [Application start script](../../deploy/scripts/start_application.sh)
- [Health validation script](../../deploy/scripts/validate_service.sh)

## 13. 운영 invariant

배포와 장애 복구 이후에도 다음 조건을 지켜야 한다.

- 모든 PR 리뷰 요청이 같은 ReviewRun 상태 머신을 사용한다.
- 한 run은 하나의 고정 base/head snapshot만 분석한다.
- 최신 head가 아닌 run은 새 publication을 만들지 않는다.
- 모든 eligible coverage item은 terminal 시 reviewed, failed 또는 deferred다.
- Partial은 complete watermark를 전진시키지 않는다.
- Same-run retry는 성공 unit을 다시 호출하지 않는다.
- Run 전체 model, tool, retry와 verifier 호출 합계는 12를 넘지 않는다.
- Exact added-line evidence를 만족하지 않는 finding은 게시하지 않는다. 위치가 틀린 finding은 같은 파일의 추가 줄에서 evidence가 유일하게 일치할 때만 재앵커한다.
- 실행 가능한 independent verifier가 실패하면 해당 candidate를 게시하지 않고 partial로 종료한다.
- Open exact occurrence만 duplicate suppression에 사용한다.
- Resolved 또는 expired occurrence는 재발 finding을 억제하지 않는다.
- Publication canonical payload와 result checkpoint가 외부 write보다 먼저 저장된다.
- Secret과 private path는 provider 입력, checkpoint, log와 publication에서 masking된다.
- TTL은 조회와 retry로 연장되지 않는다.

## 14. 검증 기준

PR #13 stress case, 작은 PR, 고위험 단일 파일 PR과 retry/stale 시나리오에서 다음을 확인한다.

- Review unit이 8개를 넘지 않고 overflow가 deferred coverage로 보인다.
- 작은 clean PR은 reviewer 한 번으로 끝난다.
- Finding이 있을 때만 independent verifier가 실행된다.
- Successful unit checkpoint는 queue retry에서 재사용된다.
- Malformed reviewer response가 다음 eligible provider로 fallback한다.
- Verifier 실패는 clean 신호가 아니라 partial이며 finding을 게시하지 않는다.
- Exact evidence가 아닌 JVM target 단정은 게시되지 않는다.
- 잘못된 line은 거리나 유사도로 이동하지 않고 같은 파일에 유일한 exact evidence가 있을 때만 재앵커한다.
- 같은 root cause의 occurrence가 결정론적으로 병합된다.
- 앞줄 삽입으로 이동한 기존 occurrence가 재앵커되고 중복 게시되지 않는다.
- 같은 evidence의 추가 발생은 신규 occurrence로 게시할 수 있다.
- 수정 후 exact 재도입된 occurrence가 reopen된다.
- Review, summary와 reply의 응답 유실이 stored canonical result와 marker로 수렴한다.
- 동일 run의 review가 중복 게시되지 않는다.
- Stale publication이 무효화되거나 bounded invalidation debt로 남는다.
- Review 본문은 내부 coverage 카운터와 unit 번호를 노출하지 않고, 미검토 범위가 있을 때만 bounded 사유와 예시를 표시한다.
- Worker 2, units 8, calls 12, findings 25와 read_file 6 상한이 유지된다.
- 120~180일 hard TTL과 daily cleanup이 expired data를 중복 억제에서 제거한다.
- Main CD가 old container 제거 후 새 container health 성공으로만 완료된다.

## 15. 문서 변경 원칙

- 구현과 이 문서가 달라지면 같은 변경에서 문서를 갱신한다.
- 상태, table, hard cap, provider model, privacy route, 보존 대상이나 외부 write 경로의 변경은 운영 invariant와 cleanup 영향을 함께 기록한다.
- 특정 PR 유형을 위한 별도 correctness 경로를 만들지 않는다.
- 비용과 장애 표면을 늘리는 구조는 측정 가능한 문제와 새로운 설계 결정을 먼저 요구한다.

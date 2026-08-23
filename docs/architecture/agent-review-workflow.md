# 확장 가능한 에이전트 리뷰 워크플로

- 설계 ID: `REVIEW-WORKFLOW-001`
- 상태: 승인된 구현 기준
- 구현 상태: 시작 전
- 최초 작성일: 2026-08-24
- 최종 갱신일: 2026-08-24
- 결정 소유자: Sandrone maintainers
- 적용 범위: 크기와 유형에 관계없는 모든 Pull Request 자동 코드 리뷰

## 1. 문서 목적

이 문서는 Sandrone의 코드 리뷰 파이프라인을 단일 장기 실행 작업에서 영속적인 다단계 워크플로로 전환하기 위한 기준을 정의한다. 이후 관련 구현은 이 문서를 기본 설계로 참조하고, 설계 결정이 바뀌면 코드보다 먼저 또는 같은 변경에서 이 문서를 갱신한다.

핵심 결정은 다음과 같다.

- 리뷰의 제어 흐름은 코드가 소유한다.
- 에이전트는 경계가 정해진 분석 작업에만 사용한다.
- 모든 리뷰 가능 hunk의 처리 상태를 영속적으로 추적한다.
- 부분 성공을 전체 성공으로 취급하지 않는다.
- 에이전트는 GitHub에 직접 게시하지 않는다.
- 분석 예산과 게시 코멘트 예산을 분리한다.
- 완료된 과거 리뷰 결과는 기본 180일, 설정 가능한 120~180일 후 제거한다.

## 2. 문제 사례

[gikipedia-server PR #13](https://github.com/wntopia/gikipedia-server/pull/13)은 현재 구조의 한계가 변경량이 많을 때 크게 드러난 재현 사례다. 릴리스 PR이라는 성격은 이 사례의 본질이 아니다. 상태, 검증, 멱등성, 증분 처리와 보존 정책은 PR 크기와 무관하게 필요하므로 새 구조는 모든 PR에 적용한다.

- 전체 변경은 159개 파일, +5,899/-906줄이다.
- 제거 파일과 patch가 없는 파일을 제외한 리뷰 가능 파일은 127개다.
- 실제 성공 결과에 포함된 파일은 25개로 리뷰 가능 파일의 약 19.7%다.
- 87개 파일은 파일 수 상한으로 모델 호출 전에 제외됐다.
- 선택된 파일 중 15개는 모델 호출 실패가 발생한 배치와 함께 제외됐다.
- 미검토 파일의 변경량은 +3,336/-42줄로 전체 변경량의 약 절반이다.
- 최종 인라인 지적은 2개뿐이고, 리뷰 본문의 대부분은 102개 미검토 파일 목록이다.

[게시된 리뷰](https://github.com/wntopia/gikipedia-server/pull/13#pullrequestreview-5002638113)는 커버리지에 비해 전체 변경을 확정적으로 설명한다. 핵심 동기화 경로 일부를 검토하지 못했음에도 데이터 일관성을 확보했다고 요약하며, 검토에 성공한 경로에서는 확장성 문제를 지적한다.

두 인라인 지적은 검증 단계의 필요성도 보여준다.

- [전체 데이터를 메모리에 적재하는 정합성 배치](https://github.com/wntopia/gikipedia-server/pull/13#discussion_r3838779100)는 코드 근거가 있는 확장성 문제다.
- [JVM target 불일치 지적](https://github.com/wntopia/gikipedia-server/pull/13#discussion_r3838779103)은 배포 대상과 도구 의미를 확인하지 않은 결론이다. 빌드에 사용할 JVM과 생성할 bytecode target은 의도적으로 다를 수 있다.

이 사례에서 해결해야 하는 문제는 모델 context 크기 하나가 아니다. 작업 분할, 상태 보존, 실패 격리, 전체 변경 합성, 근거 검증, 중복 제거, 게시 정책과 merge gate를 함께 다뤄야 한다.

## 3. 현재 구조와 실패 원인

### 3.1 하나의 긴 작업

Webhook은 `ReviewJob` 하나를 enqueue하고, worker는 하나의 `ReviewPullRequest.Execute`에서 수집, 모델 호출, 집계, 게시와 상태 저장을 모두 수행한다.

- Queue 작업은 최대 20분 동안 실행되고 전체 작업 단위로 재시도된다.
- 성공한 배치의 checkpoint가 없어서 전체 작업 재시도 시 완료된 모델 호출도 다시 수행할 수 있다.
- 동일 PR과 head에 대한 작업 고유 키나 PR별 lease가 없다.

관련 코드:

- [Queue client](../../internal/adapter/outbound/queue/asynq/client.go#L37-L51)
- [Review worker](../../internal/adapter/inbound/worker/reviewhandler.go#L24-L34)
- [Worker concurrency](../../internal/bootstrap/application.go#L187-L198)

### 3.2 파일 상한과 의미 없는 분할

기본 설정은 최대 40개 파일과 4개 배치다. 파일은 변경량이 큰 순서로 최대 개수를 선택한 후 경로순으로 다시 정렬된다. 이 방식은 작지만 위험한 인증, 권한, 트랜잭션, 설정 파일보다 큰 테스트나 문서 파일을 우선할 수 있다.

배치는 파일의 patch와 원문 문자 수만 보고 나뉜다. 구현, 인터페이스, 호출부, 설정, migration과 테스트 사이의 의미적 관계는 고려하지 않는다.

관련 코드:

- [기본 리뷰 제한](../../internal/core/setting/repoconfig.go#L25-L51)
- [파일 선택](../../internal/core/selection/fileselector.go#L17-L41)
- [배치 분할](../../internal/core/batching/batcher.go#L12-L47)

### 3.3 부분 성공이 전체 성공으로 저장됨

첫 배치가 실패하면 전체 작업을 실패시키지만 두 번째 이후 배치의 호출 또는 parsing 실패는 실패 파일 목록에 넣고 계속 진행한다. 파일 수와 배치 수 상한으로 제외된 파일도 안내 목록에만 들어간다.

게시가 성공하면 다음 동작이 수행된다.

- 리뷰 결과를 `succeeded`로 저장한다.
- 일부 파일을 검토하지 못했어도 전체 head SHA를 `LastReviewedSHA`로 저장한다.
- 다음 incremental review는 해당 SHA 이후 변경만 수집한다.

따라서 부분 리뷰에서 누락된 파일은 자동 재검토 대상에서 영구적으로 빠질 수 있다.

관련 코드:

- [배치 실패 처리와 집계](../../internal/core/usecase/reviewpullrequest/usecase.go#L91-L150)
- [성공 저장과 head 갱신](../../internal/core/usecase/reviewpullrequest/usecase.go#L182-L194)
- [증분 파일 수집](../../internal/core/usecase/reviewpullrequest/usecase.go#L208-L223)

### 3.4 Stale head와 동시 실행

Webhook delivery ID는 중복 제거하지만 `repository + PR + head SHA` 단위의 실행 고유성은 보장하지 않는다. 여러 push가 빠르게 발생하면 같은 PR의 작업이 전역 worker에서 동시에 실행될 수 있다.

작업에 head SHA가 이미 있으면 실행 시 최신 PR head로 갱신하지 않는다. 지연된 작업은 현재 PR의 파일 목록을 읽으면서 과거 SHA의 원문을 요청하고, 과거 commit을 대상으로 리뷰를 게시할 수 있다. 구작업이 늦게 끝나면 마지막 리뷰 SHA가 과거로 회귀할 수도 있다.

관련 코드:

- [Webhook routing](../../internal/adapter/inbound/webhook/eventrouter.go#L52-L78)
- [Webhook delivery 중복 제거](../../internal/adapter/inbound/webhook/handler.go#L43-L52)
- [Head 초기화](../../internal/core/usecase/reviewpullrequest/usecase.go#L33-L43)

### 3.5 Prompt에서 실제 diff가 잘릴 수 있음

모든 리뷰 가능 파일의 inventory와 프로젝트 규칙을 각 배치에 반복해서 넣고 실제 diff는 prompt의 뒷부분에 배치한다. 예약된 고정 context가 전체 예산보다 커지면 batcher는 오류로 처리하지 않고 파일 예산을 전체 예산으로 되돌린다. 최종 context renderer는 완성된 문자열의 뒤를 자르므로 실제 diff가 사라지고 inventory와 규칙만 남을 수 있다.

관련 코드:

- [Context 구성 순서와 절단](../../internal/core/prompt/context.go#L21-L30)
- [전체 파일 inventory](../../internal/core/prompt/context.go#L55-L66)
- [실제 변경 파일 배치 위치](../../internal/core/prompt/context.go#L93-L114)
- [예약 예산 처리](../../internal/core/batching/batcher.go#L16-L21)

### 3.6 전체 변경 합성 단계가 없음

각 배치는 다른 배치의 실제 diff를 보지 못한다. 성공한 finding과 파일 설명은 단순히 이어 붙이고, 첫 번째 배치의 overview만 전체 overview로 사용한다. API 계약, schema 변경과 소비 코드, transaction과 event 처리처럼 여러 파일에 걸친 문제를 최종 단계에서 검토하지 않는다.

`read_file` executor 하나를 모든 배치가 공유하므로 기본 6회의 추가 읽기 예산을 앞 배치가 소진할 수 있다.

관련 코드:

- [결과 단순 집계](../../internal/core/usecase/reviewpullrequest/usecase.go#L123-L150)
- [공유 tool executor 생성](../../internal/core/usecase/reviewpullrequest/usecase.go#L84)
- [읽기 예산 차감](../../internal/adapter/outbound/forge/github/readfileexecutor.go#L41-L51)

### 3.7 위치와 중복 검증이 약함

모델이 반환한 줄이 commentable line이 아니면 가장 가까운 변경 줄로 자동 이동한다. 위치를 정확히 찾지 못한 지적이 무관한 hunk에 붙을 수 있다.

중복 fingerprint는 파일, severity, 자연어 title과 body로 만들어진다. 같은 문제의 표현이 바뀌면 중복을 잡지 못하고, 같은 파일의 다른 위치에서 같은 문구가 나오면 서로 다른 occurrence를 구분하지 못한다. 수정 후 재발한 문제도 과거 자연어와 같으면 계속 억제될 수 있다.

관련 코드:

- [위치 자동 이동](../../internal/core/mapping/positionmapper.go#L26-L48)
- [자연어 fingerprint](../../internal/core/review/fingerprint.go#L12-L20)

### 3.8 게시가 단일 실패 지점임

모든 인라인 코멘트와 요약을 한 번의 GitHub `CreateReview` 요청으로 제출한다. 한 위치가 유효하지 않으면 전체 요청을 일반 issue comment 하나로 바꾼다. GitHub가 게시를 완료했지만 응답이 유실되면 queue retry가 같은 리뷰를 다시 게시할 수 있다.

관련 코드:

- [GitHub review 제출](../../internal/adapter/outbound/forge/github/reviewpublisher.go#L21-L51)
- [게시 실패 fallback](../../internal/core/usecase/reviewpullrequest/usecase.go#L260-L281)

### 3.9 부분 성공을 관측하기 어려움

현재 지표는 전체 작업의 성공, 실패와 실행 시간을 중심으로 한다. 후반 배치 실패를 삼키면 모니터링에는 성공으로 보인다. 파일과 hunk coverage, unit 실패와 재시도, stale run, 게시 중복 방지, verifier 기각률을 알 수 없다.

## 4. 설계 목표

### 4.1 필수 목표

- 모든 리뷰 가능 hunk에 최종 처리 상태가 있어야 한다.
- 완료, 부분 완료, 실패, 최신 head에 의해 대체된 실행을 구분해야 한다.
- 한 unit의 실패가 다른 unit의 성공 결과를 무효화하지 않아야 한다.
- 최신 head가 아닌 결과는 게시하지 않아야 한다.
- 모델의 지적은 실제 코드와 변경 hunk에 대한 근거를 가져야 한다.
- 전체 변경의 관계를 검토하는 reduce 단계를 가져야 한다.
- 분석량과 공개할 코멘트 수를 독립적으로 제어해야 한다.
- queue retry와 게시가 멱등적이어야 한다.
- GitHub와 LLM rate limit 안에서 저장소별 공정성을 보장해야 한다.
- 과거 리뷰 결과와 코드 유래 데이터는 보존 기간이 지나면 제거해야 한다.

### 4.2 비목표

- 파일마다 독립된 자율 에이전트를 무제한 생성하지 않는다.
- 에이전트에게 queue, retry, 완료 판정이나 게시 권한을 주지 않는다.
- 고정된 비용과 시간 안에 크기 제한 없는 PR을 항상 완료한다고 보장하지 않는다.
- 낮은 신뢰도의 지적을 커버리지 수치를 높이기 위해 게시하지 않는다.
- 오래된 자연어 리뷰를 영구 지식 저장소로 사용하지 않는다.
- 특정 종류의 PR만을 위한 별도 파이프라인을 만들지 않는다.

## 5. 핵심 결정

Sandrone은 자유로운 swarm이 아니라 코드가 통제하는 bounded map-verify-reduce workflow를 사용한다.

```text
Webhook
  -> immutable ReviewRun
  -> manifest / change graph
  -> ReviewUnit planner
  -> N x Unit Reviewer
  -> candidate verifier
  -> cross-unit reducer
  -> deterministic dedupe / rank / comment budget
  -> idempotent GitHub publisher and Check
       <-> coverage ledger / retry / retention
```

### 5.1 코드가 소유하는 영역

- Webhook inbox와 run 생성
- Head SHA 고정과 stale 판정
- GitHub pagination, diff와 source snapshot
- Hunk와 symbol 식별자 생성
- 작업 분할의 안전 한계와 queue 등록
- Lease, heartbeat, retry, timeout과 rate limit
- Coverage 상태 전이와 완료 판정
- Output schema와 위치 검증
- 중복 키와 게시 예산 적용
- GitHub 게시, Check 갱신과 outbox
- 보존 기간 계산과 데이터 삭제

### 5.2 에이전트를 사용할 영역

- 변경 manifest를 보고 의미 단위와 위험도를 보완하는 planning
- 제한된 unit 내부의 코드 리뷰
- 필요한 주변 symbol과 파일 선택
- 보안, 인증, 동시성, transaction, persistence 같은 전문 분석
- 여러 unit 사이 계약과 불변식 검토
- Candidate finding에 대한 독립적인 근거 평가
- 검증된 finding의 root cause 단위 병합과 요약

Planner의 판단은 coverage 항목을 조용히 제외할 수 없다. 에이전트가 선택하지 않은 hunk도 `deferred`, `skipped` 또는 다른 명시적 상태를 가져야 한다.

### 5.3 모든 PR에 적용하는 단일 파이프라인

PR 크기에 따라 기존 방식과 agentic 방식을 선택하는 별도 코드 경로를 만들지 않는다. 모든 PR은 다음 논리 단계를 거친다.

```text
ReviewRun -> Plan -> ReviewUnit -> Verify -> Reduce -> Publish -> Retain/Expire
```

규모와 위험도는 단계의 존재가 아니라 실행 계획을 바꾼다.

| 변경 특성 | 실행 계획 예시 |
| --- | --- |
| 작고 위험도가 낮은 변경 | ReviewUnit 1개, reviewer 1개, candidate가 있을 때만 verifier 실행, 단일 unit reduce |
| 일반적인 기능 변경 | 의미 단위 ReviewUnit 1~3개, unit reviewer, finding verifier, 전체 reduce |
| 넓거나 위험도가 높은 변경 | 여러 unit의 제한된 병렬 실행, 분야별 specialist, cross-unit integration review |

한 줄짜리 인증 또는 권한 변경은 파일이 작아도 심층 검토할 수 있고, 수십 개의 생성 파일 변경은 크더라도 낮은 비용으로 분류할 수 있다. 따라서 실행 강도는 파일 수 하나가 아니라 위험도, dependency 범위, 변경 종류와 token 예산으로 결정한다.

모든 실행 계획은 다음 불변식을 동일하게 지킨다.

- 고정된 head SHA의 ReviewRun을 생성한다.
- 모든 eligible hunk에 coverage 상태를 남긴다.
- Finding은 실제 hunk와 근거를 검증한다.
- 게시 전 stale head와 중복 publication을 확인한다.
- Complete와 partial을 구분한다.
- 같은 incremental과 retention 정책을 적용한다.

작은 PR의 fast path는 이 단계를 우회하는 별도 파이프라인이 아니라 unit 수와 호출 수가 1로 축약된 동일 상태 머신이다.

### 5.4 규범적 결정

이후 구현 PR과 코드에서는 필요한 경우 아래 결정 ID를 참조한다.

| ID | 결정 |
| --- | --- |
| `D-01` | MySQL을 workflow 상태의 source of truth로 사용하고 Asynq와 Redis는 전달, lease와 짧은 cache에 사용한다. |
| `D-02` | 모든 PR에 동일한 ReviewRun 상태 머신을 적용하고 규모와 위험도는 실행 계획만 바꾼다. |
| `D-03` | ReviewRun은 생성 시 고정한 base/head SHA snapshot만 읽는다. |
| `D-04` | 모든 eligible coverage item이 reviewed, deep-reviewed 또는 유효한 reused 상태일 때만 complete다. |
| `D-05` | Failed 또는 deferred item이 남으면 partial이며 complete watermark를 전진시키지 않는다. |
| `D-06` | ReviewUnit은 파일 개수가 아니라 의미 관계와 제한된 context 예산으로 구성한다. |
| `D-07` | Agent는 read-only snapshot에서 구조화된 후보만 만들고 queue, database와 GitHub를 직접 변경하지 않는다. |
| `D-08` | 공개 finding은 구조 검증과 독립 verifier를 통과해야 한다. |
| `D-09` | `MaxInlineComments`는 게시 상한이며 분석 coverage 상한이 아니다. |
| `D-10` | 결과 재사용은 immutable patch/context와 정책 version이 일치하고 원본 TTL이 유효할 때만 허용한다. |
| `D-11` | Intake, unit 실행, fan-in, publication과 watermark 전이는 멱등적이어야 한다. |
| `D-12` | 게시 직전에 head를 검증하고 stale run은 superseded 처리한다. |
| `D-13` | 기존 Go, Asynq와 MySQL을 유지하며 별도 agent framework 도입을 선행 조건으로 삼지 않는다. |
| `D-14` | 구조화된 리뷰 결과는 기본 180일, 설정 가능한 120~180일의 sliding이 아닌 hard TTL을 가진다. |
| `D-15` | Review completeness는 GitHub Check로 노출하며 partial은 merge gate를 성공시킬 수 없다. |
| `D-16` | Source, prompt와 raw response는 구조화된 리뷰 결과보다 훨씬 짧게 보존하거나 기본적으로 저장하지 않는다. |
| `D-17` | Provider와 model은 고정하지 않고 무료로 실제 운영 가능한 후보를 공통 capability와 품질 benchmark로 검증한 후 역할별 catalog에 등록한다. |

## 6. 도메인 모델

### 6.1 ReviewRun

하나의 고정된 PR head를 검토하는 최상위 실행이다.

고유 키:

```text
installation + repository + pull request number + head SHA + config hash + prompt version
```

주요 속성:

- Base와 head SHA
- Trigger와 invoker
- 설정, prompt, model policy 버전
- 상태와 terminal timestamp
- 예상 및 실제 token 비용
- 전체, 검토, 검증, 실패, deferred hunk 수
- 최신 heartbeat와 오류 요약

### 6.2 ReviewUnit

독립적으로 검토하고 재시도할 수 있는 의미적 변경 단위다. 파일 개수는 unit의 정체성이 아니다.

주요 속성:

- Run ID와 deterministic unit hash
- 포함 hunk와 관련 symbol
- 직접 dependency와 필요한 주변 context
- 위험도와 specialist 종류
- 예상 token과 tool budget
- 상태, 시도 횟수, lease와 heartbeat
- Provider, model과 사용량
- 실패 원인과 split parent

### 6.3 CoverageItem

Coverage의 기준은 파일이 아니라 실제 모델 입력에 포함된 hunk 또는 symbol hash다.

상태:

- `indexed`: 변경 항목으로 인식됨
- `planned`: unit에 배정됨
- `reviewed`: reviewer 입력에 포함되고 결과가 유효함
- `deep_reviewed`: specialist 또는 integration 검토까지 완료됨
- `failed`: 재시도 후에도 검토하지 못함
- `deferred`: 예산이나 정책에 따라 다음 실행으로 이월됨
- `skipped`: 생성물, binary 등 정책상 제외됨
- `superseded`: 새 head에서 더 이상 현재 항목이 아님

파일을 열었다는 사실만으로 파일 전체를 reviewed로 계산하지 않는다. 일부만 전달된 파일은 hunk별 상태를 유지한다.

### 6.4 CandidateFinding

Reviewer가 제출한 원본 지적이다. 공개 가능한 finding이 아니다.

필수 필드:

- Category와 rule ID
- 영향받는 symbol
- Head SHA, path, side, line과 end line
- Hunk hash와 짧은 code excerpt hash
- 문제가 발생하는 전제
- 실제 호출 또는 데이터 흐름
- 영향과 수정 방향
- 관련 파일과 symbol
- Reviewer confidence
- Reviewer model과 unit ID

### 6.5 VerifiedFinding

구조 검증과 독립 verifier를 통과한 finding이다.

Verifier 판정:

- `supported`: 근거가 충분함
- `unsupported`: 코드나 도구 의미와 맞지 않음
- `uncertain`: 추가 근거가 필요함
- `stale`: 현재 head에서 근거가 사라짐

`supported` finding만 기본 게시 후보가 된다. `uncertain`은 후속 검토를 한 번 요청할 수 있으며 근거를 확보하지 못하면 게시하지 않는다.

### 6.6 Publication

GitHub에 게시한 payload와 결과를 추적하는 outbox 항목이다.

- Run ID와 payload hash
- 숨김 marker
- GitHub review 또는 comment ID
- 시도 상태와 오류
- 대상 head SHA
- 게시 전후 확인 시각

## 7. 상태 전이

### 7.1 ReviewRun 상태

```text
planning
  -> running
  -> verifying
  -> reducing
  -> publishing
  -> complete
```

종료 또는 예외 상태:

- `partial`: 성공 결과가 있지만 failed 또는 deferred coverage가 남음
- `failed`: 유효한 리뷰 결과를 만들지 못함
- `superseded`: 더 최신 head의 run이 생성됨
- `cancelled`: 운영자 또는 명시적 정책으로 취소됨
- `skipped`: 모든 변경이 명시적 정책에 따라 분석 대상이 아님

`partial`, `failed`, `superseded`, `cancelled`는 complete watermark를 전진시키지 않는다. `skipped`는 coverage denominator가 0이고 모든 제외 사유가 기록된 경우에만 정상 종료로 인정한다. `partial`의 성공 unit은 보존하고 실패 또는 deferred unit만 다시 예약할 수 있어야 한다.

### 7.2 ReviewUnit 상태

```text
pending -> leased -> running -> succeeded
                    |        -> split
                    |        -> retry_wait -> pending
                    |        -> failed
                    -> superseded
```

Unit 상태 변경은 compare-and-swap 또는 row lock으로 보호한다. Worker crash 후 lease가 만료되면 다른 worker가 재개할 수 있어야 한다.

## 8. 수집과 snapshot

### 8.1 Durable inbox

Webhook delivery 중복 키를 소비하기 전에 이벤트와 run 생성 의도를 영속화한다. Queue enqueue가 실패하면 inbox dispatcher가 다시 시도할 수 있어야 한다. Redis delivery dedupe만으로 이벤트 수신 완료를 판단하지 않는다.

### 8.2 Immutable head

ReviewRun은 생성 시 head SHA를 고정한다. 모든 diff, 파일 내용, tool read와 게시 위치는 같은 SHA를 사용한다.

- 새 push 이벤트는 짧게 debounce하고 최신 head를 우선한다.
- 새 run이 생성되면 이전 비종료 run은 `superseded` 후보가 된다.
- Worker는 장기 호출 전후와 게시 직전에 최신 head를 확인한다.
- Stale run은 GitHub에 게시하지 않는다.

### 8.3 공유 snapshot

각 에이전트가 GitHub Contents API를 직접 호출하지 않는다. Coordinator가 head SHA 기준으로 read-only snapshot 또는 content-addressed cache를 만들고 unit worker가 공유한다.

- Diff, source와 instruction 문서는 한 번 읽고 hash를 기록한다.
- GitHub token은 agent context나 sandbox에 전달하지 않는다.
- Cache miss는 중앙 repository content adapter를 통해 rate limit을 지키며 채운다.
- Patch가 잘리거나 GitHub API에서 빠졌다는 사실을 coverage와 prompt에 표시한다.

## 9. Planning과 ReviewUnit 구성

### 9.1 결정론적 전처리

Planner 전에 코드가 다음 정보를 만든다.

- 변경 파일과 hunk 목록
- 언어와 생성물, binary 여부
- Import, symbol, call과 module dependency
- 변경 commit과 co-change 관계
- 구현과 테스트 관계
- 설정, migration, 권한, 외부 API 변경 여부
- 예상 token과 위험 신호

삭제, rename, binary, submodule, 생성물과 patch unavailable 항목도 manifest에서 사라지지 않는다. 삭제 파일은 inline anchor가 없더라도 계약 파손 분석과 summary finding의 대상이 될 수 있다. 정책상 분석할 수 없는 항목은 coverage에 명시적인 eligibility와 reason을 남긴다.

### 9.2 의미 단위 우선순위

Unit은 다음 기준을 조합해 구성한다.

1. 같은 기능과 module/package 경계
2. Interface와 구현, 호출부
3. Entity, repository, migration과 transaction
4. Endpoint, DTO, validation과 client
5. 구현 코드와 직접 검증하는 테스트
6. 설정과 해당 설정을 소비하는 코드
7. Commit 또는 기존 PR 경계

기존 PR 경계는 여러 신호 중 하나일 뿐이며 별도 릴리스 전용 경로를 만들지 않는다.

### 9.3 큰 unit 분할

Unit이 provider 예산을 넘으면 파일 개수로 자르지 않고 symbol 또는 hunk 단위로 나눈다. Split된 unit은 공통 contract summary를 공유하며 최종 integration pass에서 다시 연결한다.

### 9.4 위험도

다음 변경은 우선순위와 검증 깊이를 높인다.

- 인증, 권한, 암호와 secret 처리
- Transaction, lock, concurrency와 event ordering
- Schema와 migration
- 데이터 삭제와 복구
- Cache consistency와 distributed state
- 외부 API, storage와 network boundary
- Build, 배포와 runtime 설정
- 넓은 호출 범위의 public contract

큰 파일이라는 이유만으로 높은 우선순위를 부여하지 않는다.

## 10. Unit Reviewer

Reviewer는 전체 파일 inventory를 반복해서 받지 않는다. 다음 context만 제공한다.

- Unit의 실제 diff와 hunk ID
- 해당 symbol의 현재 내용
- 직접 dependency와 호출자 일부
- 적용 가능한 프로젝트 규칙
- 전체 PR의 짧은 목적과 contract summary
- 도구 정의와 unit별 예산

고정 지침과 schema는 안정된 prefix로 유지하고 변경 데이터는 뒤에 둔다. 실제 diff가 보장된 예산을 먼저 확보한 후 부가 context를 채운다. Context가 부족하면 부가 정보부터 축약하며 diff를 조용히 절단하지 않는다.

Reviewer가 사용할 수 있는 도구:

- Head SHA에 고정된 파일과 symbol 읽기
- Repository 내부 검색
- 의존 관계 조회
- 격리된 build, test와 정적 분석 요청
- 필요한 경우 신뢰 가능한 공식 문서 조회 요청

각 unit은 독립된 tool budget을 가진다. 에이전트는 shell, GitHub 쓰기, network credential 또는 게시 도구를 직접 사용할 수 없다.

## 11. 근거 검증

검증은 다음 순서로 진행한다.

1. Finding의 head SHA가 run과 같은지 확인한다.
2. Path와 excerpt가 해당 head에 실제 존재하는지 확인한다.
3. Hunk hash와 line이 실제 변경 hunk의 commentable line인지 확인한다.
4. 주장한 symbol, 호출 관계와 데이터 흐름이 코드에 존재하는지 확인한다.
5. 독립 verifier가 발생 조건과 영향을 재평가한다.
6. 고위험 지적은 가능한 경우 build, test, 정적 분석 또는 공식 문서로 보강한다.

잘못된 줄을 가장 가까운 변경 줄로 자동 이동하지 않는다. 정확한 anchor를 다시 요청하고, 찾지 못하면 일반 summary 후보로 낮추거나 기각한다.

PR 코드는 신뢰할 수 없는 입력이다. 실행 검증은 network를 차단하고 CPU, memory와 시간을 제한한 일회성 sandbox에서 수행한다. GitHub, LLM, database credential은 sandbox에 주입하지 않는다.

## 12. Reduce와 통합 검토

Reducer는 성공 unit의 압축 summary, verified finding, contract와 coverage를 입력으로 받는다. 모든 원본 diff를 다시 한 prompt에 넣지 않는다.

Reducer의 책임:

- Unit 사이 API와 schema 계약 확인
- Transaction, event와 state ordering 확인
- 같은 root cause에서 파생된 finding 병합
- 서로 모순되는 finding 해결
- 전체 변경 overview 재작성
- 누락된 관계가 있으면 제한된 follow-up unit 생성
- 최종 우선순위와 게시 예산 적용

Follow-up은 횟수와 token 예산이 제한된 명시적 작업이어야 한다. Reducer가 무제한으로 새 작업을 만들 수 없다.

## 13. Finding identity와 lifecycle

자연어 본문 hash 하나로 중복을 판단하지 않는다.

```text
issue_key = repository + category/rule + normalized symbol + root cause signature
occurrence_key = issue_key + hunk/context hash + exact evidence anchor
```

- 같은 root cause의 여러 증상은 하나의 issue로 병합한다.
- 같은 issue가 다른 위치에서 발생하면 occurrence를 구분한다.
- 새 head에서는 기존 open finding의 근거를 다시 검증한다.
- 근거가 사라지면 `resolved` 또는 `superseded`로 전환한다.
- 수정 후 재발하면 새 occurrence로 기록한다.
- 자연어 semantic similarity는 결정론적 후보를 좁힌 후 보조 수단으로만 사용한다.

## 14. 분석 예산과 코멘트 예산

`MaxInlineComments`는 분석을 중단하는 기준이 아니라 공개 게시 예산이다.

모든 unit은 허용된 분석 예산 안에서 검토하고 supported finding을 ranking한다. 기본 우선순위 신호:

- Severity
- Confidence와 검증 근거
- 영향 범위
- 재현 가능성
- 신규성
- 수정 가능성과 actionability
- 같은 파일 또는 domain의 코멘트 편중 여부

같은 root cause는 한 코멘트로 합친다. 낮은 신뢰도의 minor와 nit는 변경 규모와 게시 예산에 따라 summary로 접거나 생략할 수 있지만 분석 여부와 coverage는 유지한다.

## 15. 게시와 merge gate

### 15.1 게시 원칙

- Agent는 candidate만 만들고 publisher만 GitHub에 쓴다.
- 최종 GitHub review는 가능한 한 한 번 게시한다.
- 진행 상황은 반복 코멘트 대신 하나의 GitHub Check에서 갱신한다.
- Summary에는 전체 미검토 파일 표 대신 compact coverage를 표시한다.
- 상세 coverage는 dashboard 또는 Check artifact로 연결한다.

예시:

```text
대상 hunk 412개: 검토 412, 심층 검토 96, 실패 0, deferred 0
검증 후보 11개: 게시 5, root cause 병합 3, 기각 3
```

### 15.2 Check 결론

- 실행 중: `pending`
- Complete이고 blocking finding 없음: `success`
- Complete이고 설정된 blocking severity finding 존재: `failure`
- Partial 또는 failed: merge gate 모드에서는 `failure`
- Superseded: 최신 run으로 대체하고 과거 Check는 중립 또는 취소 상태

단순 `COMMENTED` review만으로 완료나 merge 가능 여부를 표현하지 않는다.

### 15.3 멱등 게시

Publication outbox와 숨김 marker를 사용한다.

1. Payload hash를 저장한다.
2. 게시 전 같은 marker와 hash가 있는지 GitHub에서 확인한다.
3. 게시 후 GitHub ID를 저장한다.
4. 응답 유실 시 다시 게시하기 전에 reconcile한다.
5. 일부 inline 위치가 잘못되면 유효한 코멘트를 보존하고 잘못된 위치만 격리한다.

## 16. 실패와 재시도 정책

실패는 unit 단위로 격리한다.

| 실패 | 처리 |
| --- | --- |
| 429, 5xx, network timeout | 해당 unit만 `Retry-After`와 jitter를 적용해 재시도 |
| Context overflow | Unit을 symbol 또는 hunk 단위로 split |
| 잘못된 output schema | 제한된 repair 1회 후 unit 재실행 |
| Worker crash | Lease 만료 후 재queue |
| Provider 장애 | Circuit breaker 후 다른 provider 사용, 완료 unit 보존 |
| Head 변경 | Run을 superseded 처리하고 게시 금지 |
| 예산 소진 | 남은 coverage를 deferred로 남기고 partial 처리 |
| 잘못된 inline anchor | 재검증 후 해당 finding만 fallback 또는 기각 |

Queue task ID는 `run ID + unit hash + attempt generation`으로 결정한다. 동일 generation의 중복 실행이 생겨도 상태 compare-and-swap과 결과 unique key로 한 번만 반영한다.

## 17. Incremental review

하나의 `LastReviewedSHA`로 전체 coverage를 표현하지 않는다.

- Complete run의 unchanged hunk 결과는 재사용할 수 있다.
- 새 head에서 변경된 hunk와 dependency context가 바뀐 unit만 invalidation한다.
- Failed와 deferred coverage는 다음 run으로 이월한다.
- 기존 open finding은 새 head에서 다시 검증한다.
- Prompt, rule, model policy 또는 verifier version이 바뀌면 재사용 가능성을 다시 계산한다.
- 정확히 같은 patch라도 주변 context hash가 바뀌면 integration 검토 대상이 된다.
- 게시 직전 run head와 현재 head가 다르면 결과를 게시하지 않는다.

재사용 키에는 최소한 다음 정보가 포함돼야 한다.

```text
patch hash + context hash + instruction hash + prompt version + review policy version
```

재사용은 비용 최적화이며 완료 판정을 우회하는 수단이 아니다. 재사용된 coverage도 현재 run에 명시적으로 기록한다.

재사용된 결과는 최초 evidence의 `origin_created_at`과 `origin_expires_at`을 함께 기록한다. 새 run에 참조됐다는 이유로 원본 evidence의 만료 시각을 갱신하거나 같은 결과를 새 artifact로 복제해 TTL을 우회하지 않는다. 원본 만료가 현재 run deadline보다 이르면 재사용하지 않고 다시 검토한다.

## 18. 데이터 보존과 제거

### 18.1 기본 정책

과거 리뷰 결과를 무기한 보존하지 않는다.

- 구조화된 리뷰 결과의 기본 보존 기간은 terminal timestamp부터 180일이다.
- 운영자는 보존 기간을 120~180일 범위에서 설정할 수 있다.
- 저장소 설정은 더 짧은 기간을 요청할 수 있지만 운영자 상한보다 늘릴 수 없다.
- 보존 기간은 마지막 조회 시점에 따라 연장하지 않는 hard TTL이다.
- 완료된 run이 아직 열린 PR에 속하더라도 TTL이 지나면 제거한다.
- `running` 상태는 TTL에서 제외하지만 run deadline과 orphan reconciler로 무기한 active 상태를 방지한다.
- Progress와 유효한 lease가 7일 동안 없는 run은 reconciler가 `failed`로 종료해 TTL 계산을 시작한다.

보존 기간이 지난 결과는 중복 억제, incremental reuse와 finding lifecycle 판단에 사용하지 않는다. 이후 이벤트가 오면 남아 있는 최신 coverage가 없는 범위를 새로 검토한다.

### 18.2 데이터 종류별 보존

| 데이터 | 기본 보존 | 정책 |
| --- | --- | --- |
| Source snapshot과 전체 파일 내용 | Run 종료 후 24시간 이내 | 공유 cache 용도만 허용하고 가능한 한 영속화하지 않음 |
| 전체 prompt와 raw model response | 기본 저장 안 함 | Debug 모드에서도 최대 30일, secret masking 필수 |
| ReviewRun, ReviewUnit, CoverageItem | 180일 | 120~180일 설정 가능, terminal timestamp 기준 |
| Candidate와 VerifiedFinding | 180일 | Source run과 함께 삭제 |
| Finding lifecycle과 reuse index | 180일 | 원본 결과보다 오래 보존하지 않음 |
| Publication과 GitHub ID | 180일 | 멱등 reconcile 기간 동안만 보존 |
| 집계 metric | 장기 보존 가능 | 코드, prompt, 자연어 finding과 저장소 식별 정보를 포함하지 않는 집계만 허용 |

GitHub에 이미 게시된 review와 comment는 외부 시스템의 기록이므로 로컬 cleanup이 삭제하지 않는다. 로컬 publication metadata가 만료된 후에는 과거 게시물을 자동 수정하거나 중복 억제 근거로 사용하지 않는다.

### 18.3 삭제 기준

Cleanup 대상은 다음 조건을 만족하는 terminal run이다.

```text
terminal_at < now - effective retention period
```

Terminal 상태는 `complete`, `partial`, `failed`, `superseded`, `cancelled`, `skipped`다. 보존 기간은 `updated_at`이나 `last_accessed_at`이 아니라 `terminal_at`을 기준으로 계산한다.

### 18.4 Cleanup 실행

- Cleanup worker는 하루 한 번 실행한다.
- 한 transaction에서 과도한 row를 삭제하지 않도록 작은 batch로 처리한다.
- Foreign key는 run 삭제 시 unit, coverage, candidate, verified finding과 publication이 함께 삭제되도록 구성한다.
- 큰 payload나 object storage artifact를 먼저 삭제하고 database row를 제거한다.
- 실패한 삭제는 idempotent하게 재시도한다.
- 실행 중인 run이나 유효한 lease가 있는 unit은 삭제하지 않는다.
- Cleanup은 GitHub 또는 LLM 호출을 수행하지 않는다.
- Database backup, log, search index와 object storage lifecycle도 같은 콘텐츠 보존 상한을 넘기지 않도록 별도 만료 정책을 둔다.

필수 cleanup 지표:

- 종류별 삭제 row와 artifact 수
- 가장 오래된 terminal run의 나이
- 보존 기간을 넘긴 미삭제 run 수
- Cleanup 실행 시간과 실패 횟수
- 삭제로 회수한 저장 공간

### 18.5 만료 후 동작

- 만료된 finding은 새 리뷰에서 자동 중복으로 취급하지 않는다.
- 만료된 hunk 결과는 재사용하지 않는다.
- 열린 PR에서 모든 관련 결과가 만료됐다면 다음 요청은 full review baseline으로 시작한다.
- GitHub의 기존 thread가 남아 있어도 로컬에서는 새 근거를 기준으로 finding을 다시 판단한다.
- 과거 run을 참조하는 dashboard 링크는 `expired` 상태를 명확히 표시한다.

## 19. Rate limit과 공정성

GitHub와 LLM에 별도 중앙 rate broker를 둔다.

### 19.1 GitHub

- Installation별 primary와 secondary rate limit 추적
- `Retry-After`와 rate-limit response header 준수
- Source snapshot과 content-addressed cache 공유
- Mutating request는 PR별 직렬화
- Agent가 직접 API를 호출하지 않음

### 19.2 LLM

- Provider, model과 account별 RPM/TPM token bucket
- 예상 token을 예약하고 실제 사용량으로 정산
- Provider cooldown과 circuit breaker
- 저장소와 installation별 weighted fair queue
- 대화형 reply, 명시적 review, 자동 background review 순으로 우선순위 설정 가능

Per-PR unit 동시성은 전체 worker concurrency와 분리한다. 초기값은 2~4개로 제한하고 운영 지표로 조정한다. 무제한 병렬화로 rate limit과 비용을 확대하지 않는다.

### 19.3 Provider와 model 등록 기준

현재 [provider catalog](../../internal/adapter/outbound/llm/provider/catalog.go)는 출발점일 뿐이며 특정 회사나 model 목록을 핵심 설계로 고정하지 않는다. 무료로 사용할 수 있는 새 provider와 model은 다음 조건을 실제 API 호출과 benchmark로 검증한 후 추가할 수 있다.

필수 조건:

- 공식 또는 운영이 허용된 API를 안정적으로 호출할 수 있음
- 자동 코드 리뷰에 사용할 수 있는 무료 quota가 실제로 제공됨
- 한국어 지시 이해와 자연스러운 한국어 리뷰 품질이 기준을 통과함
- 역할에 필요한 tool calling 또는 read/search 요청을 정확히 수행함
- 구조화 출력 또는 엄격한 JSON schema를 충분히 안정적으로 지킴
- 필요한 context와 output 한도를 제공함
- Rate limit, timeout, retry와 usage 정보를 운영 코드에서 다룰 수 있음
- 코드 전송, 학습 사용, 보존 기간과 개인정보 정책이 운영 기준을 충족함
- 모델명, endpoint와 무료 정책을 공식 자료로 확인할 수 있음

평가 항목:

- 작은 PR의 correctness와 불필요한 지적 억제
- 여러 파일의 contract와 data flow 추론
- 한국어 finding의 정확성, 밀도와 일관된 severity
- Tool call 인자, 횟수와 종료 조건 준수
- Output schema 준수율과 repair 비율
- PR #13 stress benchmark의 coverage, latency와 token 사용량
- JVM target 사례 같은 외부 사실 의존 지적의 verifier 기각 여부
- 429, 5xx, malformed response와 context overflow에서의 실패 형태

Provider capability는 model별로 기록하고 planner, reviewer, verifier와 reducer 역할별 허용 여부를 분리한다. 한 역할에서 통과한 모델을 다른 역할에 자동으로 사용하지 않는다. 특히 verifier는 reviewer와 다른 모델 또는 독립된 context로 결론을 재평가해야 한다.

무료 quota와 model availability는 바뀔 수 있으므로 정기적으로 재검증한다. 기준을 더 이상 만족하지 않는 provider는 새 배포 없이도 운영 설정으로 비활성화할 수 있어야 하며, 품질이 확인되지 않은 모델을 단순한 rate-limit fallback으로 사용하지 않는다.

## 20. 보안 경계

- PR 제목, 본문, diff와 파일 내용은 신뢰할 수 없는 입력이다.
- 프로젝트 지침은 리뷰 기준으로만 사용하고 system contract를 바꿀 수 없다.
- Secret masking은 snapshot, prompt, tool output과 저장 데이터 모두에 적용한다.
- Agent와 sandbox에 GitHub App key, database credential과 provider key를 노출하지 않는다.
- 실행 검증 sandbox는 network, CPU, memory, disk와 시간을 제한한다.
- Fork PR의 코드를 운영 환경에서 직접 실행하지 않는다.
- 게시 권한은 deterministic publisher adapter에만 둔다.

## 21. 관측 지표

최소 지표:

- Eligible hunk coverage
- Reviewed와 deep-reviewed coverage
- Complete, partial, failed와 superseded run 비율
- Unit 성공, split, retry와 실패 비율
- Candidate -> supported -> published 전환율
- Verifier unsupported와 uncertain 비율
- 사람의 thread resolve, 수정 반영과 반박 비율
- 중복 게시과 stale 게시 횟수
- P50/P95 run과 unit 지연 시간
- Provider별 token, 비용과 1,000 changed lines당 비용
- GitHub와 provider rate-limit 대기 시간
- Retention deadline을 넘긴 데이터 수

파일 수만으로 coverage를 보고하지 않는다. Partial run을 성공 지표에 포함하지 않는다.

## 22. 현재 코드에서의 전환 방향

기존 포트는 가능한 한 유지한다.

- `PullRequestSource`: manifest와 snapshot 수집에 재사용
- `Completer`: bounded reviewer, verifier와 reducer 호출에 재사용
- `ToolExecutorFactory`: unit별 executor 생성으로 변경
- `Renderer`: compact coverage와 verified finding만 렌더링하도록 확장
- `ReviewPublisher`: outbox와 Check publisher 뒤에서 재사용
- `FindingRepository`: candidate, verified finding과 lifecycle 저장소로 분리
- `PullRequestStateRepository`: 단일 SHA watermark 대신 run과 coverage 조회로 대체

현재 거대한 `ReviewPullRequest.UseCase`는 coordinator로 축소하고 다음 단계로 분리한다.

```text
CreateReviewRun
IndexPullRequest
PlanReviewUnits
ReviewUnit
VerifyFinding
ReduceReview
PublishReview
ReconcilePublication
ExpireReviewData
```

각 클래스, 인터페이스 또는 동등한 단위는 별도 파일에 둔다는 저장소 규칙을 유지한다.

예상 persistence 모델:

- `review_runs`
- `review_units`
- `coverage_items`
- `candidate_findings`
- `verified_findings`
- `finding_occurrences`
- `publications`
- `webhook_inbox`
- `workflow_outbox`
- `review_unit_dependencies`

MySQL을 상태의 source of truth로 사용하고 Redis와 Asynq는 전달, lease와 짧은 cache에 사용한다.

MySQL 상태 변경과 Asynq enqueue는 하나의 transaction이 아니므로 후속 unit, fan-in, publication과 cleanup 요청은 `workflow_outbox`에 먼저 기록한다. Dispatcher는 outbox를 멱등적으로 전달하고, reconciler는 전달이 유실돼도 database 상태에서 필요한 작업을 다시 생성한다.

## 23. 단계별 구현 계획

### 단계 1. 정확한 상태와 부분 성공

- [ ] `ReviewRun`, `ReviewUnit`, `CoverageItem` 모델 추가
- [ ] Head SHA와 config/prompt version으로 run 고유성 보장
- [ ] Complete, partial, failed와 superseded 상태 도입
- [ ] Partial에서 전체 SHA watermark를 전진시키지 않음
- [ ] PR별 lease와 stale head 게시 차단
- [ ] 보존 기간 필드와 cleanup worker를 초기 schema에 포함
- [ ] Prompt에서 실제 diff 예산을 우선 보장

### 단계 2. Durable fan-out/fan-in

- [ ] 의미 단위 planner와 deterministic unit hash 도입
- [ ] Unit별 Asynq 작업, lease, heartbeat와 checkpoint 구현
- [ ] Unit 실패 격리와 context overflow split 구현
- [ ] Unit별 tool executor와 token budget 적용
- [ ] 성공 unit을 보존하는 reducer trigger 구현

### 단계 3. 검증과 게시 안정성

- [ ] Candidate와 verified finding 분리
- [ ] Exact hunk, excerpt와 anchor 검증
- [ ] 자동 nearest-line 이동 제거
- [ ] Root cause와 occurrence 기반 중복 키 도입
- [ ] Publication outbox와 marker reconcile 구현
- [ ] Compact coverage summary와 GitHub Check 구현

### 단계 4. Agentic planning과 통합 검토

- [ ] Planner agent를 deterministic planner의 보조로 shadow 실행
- [ ] 위험 영역 specialist reviewer 도입
- [ ] Independent verifier 도입
- [ ] Cross-unit integration reducer 도입
- [ ] Follow-up unit 횟수와 예산 제한 구현
- [ ] Provider/model 역할별 capability benchmark와 등록 절차 구현
- [ ] 검증을 통과한 무료 provider/model 후보 추가

### 단계 5. Incremental reuse와 운영 최적화

- [ ] Hunk와 context hash 기반 결과 재사용
- [ ] Dependency 변경에 따른 unit invalidation
- [ ] 중앙 GitHub와 LLM rate broker 구현
- [ ] Snapshot cache와 API 요청량 최적화
- [ ] Retention cleanup과 dashboard expired UX 검증
- [ ] 비용과 품질 지표 기반 동시성 조정

## 24. 검증과 출시 기준

### 24.1 고정 사례

PR #13의 head SHA를 stress benchmark로 사용한다. 별도로 작은 변경과 고위험 단일 파일 변경도 같은 상태 머신의 benchmark에 포함한다. 모든 사례에서 다음을 검증한다.

- 리뷰 가능 hunk 전체가 terminal coverage 상태를 가짐
- Unit 실패 후 성공 unit을 다시 호출하지 않고 실패 unit만 재개함
- Partial이 success 또는 complete로 표시되지 않음
- 최신 head가 아니면 게시하지 않음
- 배포 target 근거 없는 JVM target 지적이 verifier에서 기각됨
- 전체 누락 파일 표 대신 compact coverage가 게시됨
- 최종 review가 중복 없이 한 번 게시됨

### 24.2 출시 gate

- Stale review 게시 0건
- 동일 run 중복 review 게시 0건
- Partial run의 complete watermark 갱신 0건
- Eligible hunk 상태 누락 0건
- 보존 기간을 cleanup 주기보다 오래 초과한 terminal data 0건
- Provider outage에서 성공 unit 재실행 없음
- 검증되지 않은 candidate 게시 0건
- 역할별 benchmark를 통과하지 않은 provider/model 호출 0건

### 24.3 Canary

1. 기존 결과와 새 결과를 게시 없이 비교하는 shadow mode
2. 내부 또는 선택 저장소의 명시적 review에서 사용
3. 5~10% 자동 리뷰에 canary 적용
4. False positive, coverage, 비용과 P95 지연을 확인한 후 확대

## 25. 문서 변경 원칙

- 구현이 이 문서와 달라지면 같은 변경에서 문서를 갱신한다.
- 완료된 구현 항목은 단계별 checklist에 반영한다.
- 새 상태, 테이블, 보존 대상이나 외부 쓰기 경로를 추가하면 상태 전이, 멱등성, cleanup 영향을 함께 기록한다.
- 특정 모델이나 provider의 일시적 제약은 핵심 설계에 고정하지 않고 운영 설정으로 둔다.
- 특정 PR 유형에만 맞춘 최적화는 일반 pipeline의 correctness를 우회하지 않아야 한다.
